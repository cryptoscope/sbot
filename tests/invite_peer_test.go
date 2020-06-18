// SPDX-License-Identifier: MIT

package tests

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/plugins2/bytype"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/nacl/auth"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/sbot"
)

// first js creates an invite
// go will play introducer node
// second js peer will try to use/redeem the invite
func XTestPeerInviteJSCreate(t *testing.T) {

	r := require.New(t)

	os.Remove("peer_invite.txt")

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot(
		sbot.LateOption(sbot.MountPlugin(&bytype.Plugin{}, plugins2.AuthMaster)),
		sbot.LateOption(sbot.MountSimpleIndex("get", indexes.OpenGet)),
	)
	bob := ts.gobot

	wrappedAddr := bob.Network.GetListenAddr()
	// manual multiserver address
	addr := fmt.Sprintf("net:%s", netwrap.GetAddr(wrappedAddr, "tcp").String())
	addr += "~shs:"
	addr += base64.StdEncoding.EncodeToString(bob.KeyPair.Id.ID)
	t.Log("addr:", addr)

	bob.PublishLog.Append(map[string]interface{}{
		"type":         "address",
		"availability": 1,
		"address":      addr,
	})

	createInvite := fmt.Sprintf(`
		sbot.publish({
			type: 'contact',
			following: true,
			contact: %q
		}, (err, followmsg) => {
			t.error(err)

			sbot.peerInvites.create({
				allowWithoutPubs: true,
				pubs: %q
			}, (err, invite) => {
				t.error(err)
				
				var fs = require('fs')
				fs.writeFile('peer_invite.txt', invite, (err) => {  
					t.error(err)
					
					t.comment('ssb-js: invite saved!');
					run()
					setTimeout(exit, 1000)
				});
			})
	
		})
	`, bob.KeyPair.Id.Ref(), addr)

	alice := ts.startJSBot(createInvite, ``)

	// bob is FRIENDS with alice (and thus replicating her invites)
	newSeq, err := bob.PublishLog.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   alice.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

	<-ts.doneJS

	// prelim check of invite

	invite, err := ioutil.ReadFile("peer_invite.txt")
	r.NoError(err)
	r.True(bytes.HasPrefix(invite, []byte("inv:")))
	t.Log(string(invite))

	inviteStr := strings.TrimPrefix(string(invite), "inv:")

	invData := strings.Split(inviteStr, ",")

	// use the seed to make a keypair
	seed, err := base64.StdEncoding.DecodeString(invData[0])
	r.NoError(err)
	r.Equal(32, len(seed))
	seedKp, err := ssb.NewKeyPair(bytes.NewReader(seed))
	r.NoError(err)

	// bob has the message
	invRef, err := refs.ParseMessageRef(invData[1])
	r.NoError(err)
	msg, err := bob.Get(*invRef)
	r.NoError(err)

	// can verify the invite message
	enc, err := legacy.EncodePreserveOrder(msg.ContentBytes())
	r.NoError(err)
	invmsgWoSig, sig, err := legacy.ExtractSignature(enc)
	r.NoError(err)

	peerCapData, err := base64.StdEncoding.DecodeString("HT0wIYuk3OWc2FtaCfHNnakV68jSGRrjRMP9Kos7IQc=") // hash("peer-invites")
	r.NoError(err)

	r.Equal(32, len(peerCapData))
	var peerCap [32]byte
	copy(peerCap[:], peerCapData)

	mac := auth.Sum(invmsgWoSig, &peerCap)
	err = sig.Verify(mac[:], seedKp.Id)
	r.NoError(err)

	// invite data matches
	var invCore struct {
		Invite *refs.FeedRef `json:"invite"`
		Host   *refs.FeedRef `json:"host"`
	}
	err = json.Unmarshal(invmsgWoSig, &invCore)
	r.NoError(err)
	r.Equal(alice.ID, invCore.Host.ID)
	r.Equal(seedKp.Id.ID, invCore.Invite.ID)
	t.Log("invitee key:", seedKp.Id.Ref())

	// 2nd node does it's dance
	before := fmt.Sprintf(`
	var fs = require('fs')
	fs.readFile('invite.txt', 'utf8', (err, invite) => {
		t.error(err)
		t.comment("opened invite:"+invite)
		sbot.peerInvites.openInvite(invite, (err, inv_msg, content) => {
			t.error(err)

			t.comment('invMsg:'+JSON.stringify(inv_msg))
			t.comment('content:'+JSON.stringify(content))
			// TODO: check reveal/private?

			sbot.peerInvites.acceptInvite(invite, (err) => {
				t.error(err)
				t.comment('accepted invite')
				// is now able to connect with its longterm
				run()
			})
		})
	})
	`)

	after := fmt.Sprintf(`aliceFeed = %q // global - pubKey of the first alice
bobFeed = %q
sbot.on('rpc:connect', (rpc) => {
	t.comment('now should have feed:' + aliceFeed)
	rpc.on('closed', () => { 
		t.comment('now should have feed:' + aliceFeed)
		pull(
			sbot.createUserStream({id: aliceFeed }),
			pull.collect(function(err, msgs) {
				t.error(err, 'query worked')
				t.equal(2, msgs.length, 'got all the messages')

				checkBob()
			})
		)

		function checkBob() {
			pull(
				sbot.createUserStream({id: bobFeed }),
				pull.collect(function(err, msgs) {
					t.error(err, 'query worked')
					t.equal(4, msgs.length, 'got all the messages')
	console.warn("accept",JSON.stringify(msgs[2]))
					exit()
				})
			)
		}
	})
})
`, alice.Ref(), bob.KeyPair.Id.Ref())

	ts.startJSBot(before, after)
	// time.Sleep(2 * time.Second)
	log.Println("[TEST] waited, blocking on close done")
	<-ts.doneJS
	log.Println("[TEST] done chan closed")

	// reuse
	// 2nd node does it's dance
	reuseBefore := fmt.Sprintf(`
		var fs = require('fs')
		fs.readFile('invite.txt', 'utf8', (err, invite) => {
			t.error(err)
			t.comment('cant use again')
			sbot.peerInvites.openInvite(invite, (err, inv_msg, content) => {
				t.ok(err)
				t.comment(err)
				t.true(typeof inv_msg === 'undefined')
				t.true(typeof content === 'undefined')
				exit()
			})
		})
		`)
	ts.startJSBot(reuseBefore, ``)

	ts.wait()
}
