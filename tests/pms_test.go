// SPDX-License-Identifier: MIT

package tests

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/private"
	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
)

func TestPrivMsgsFromGo(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	testRepo := repo.New(ts.repo)
	aliceKP, err := repo.DefaultKeyPair(testRepo)
	r.NoError(err)

	mlogPriv := multilogs.NewPrivateRead(log.With(ts.info, "module", "privLogs"), aliceKP)

	ts.startGoBot(
		sbot.LateOption(sbot.MountMultiLog("privLogs", mlogPriv.OpenRoaring)),
	)
	s := ts.gobot

	before := `fromKey = testBob

	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => { 
			t.comment('now should have feed:' + fromKey)
			pull(
				sbot.private.read({}),
				pull.collect(function(err, msgs) {
					t.error(err, 'private read worked')
					t.equal(msgs.length, 6, 'got all the messages')

					t.equal(msgs[0].value.sequence, 2, 'sequence:0')
					t.deepEqual(msgs[0].value.content, [1,2,3,4,5], 'sequence:0 val')

					t.equal(msgs[1].value.sequence, 3, 'sequence:1')
					t.equal(msgs[1].value.content.some, 1, 'sequence:1 val')

					t.equal(msgs[2].value.sequence, 4, 'sequence:2')
					t.equal(msgs[2].value.content.hello, true, 'sequence:2 val')

					t.equal(msgs[3].value.sequence, 5, 'sequence:3')
					t.equal(msgs[3].value.content, "plainStringLikeABlob", 'sequence:3 val')

					t.equal(msgs[4].value.sequence, 6, 'sequence:4')
					t.equal(msgs[4].value.content.hello, false, 'sequence:4 val')

					t.equal(msgs[5].value.sequence, 7, 'sequence:5')
					t.equal(msgs[5].value.content.hello, true, 'sequence:5 val')
					exit()
				})
			)
		})
	})
	
	sbot.publish({type: 'contact', contact: fromKey, following: true}, function(err, msg) {
		t.error(err, 'follow:' + fromKey)

		sbot.friends.get({src: alice.id, dest: fromKey}, function(err, val) {
			t.error(err, 'friends.get of new contact')
			t.equals(val[alice.id], true, 'is following:'+JSON.stringify(val))

			t.comment('shouldnt have bobs feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey}),
				pull.collect(function(err, vals){
					t.error(err)
					t.equal(0, vals.length)
					sbot.publish({type: 'about', about: fromKey, name: 'test bob'}, function(err, msg) {
						t.error(err, 'about:' + msg.key)
						run()
					})
				})
			)

}) // friends.get

}) // publish`

	alice := ts.startJSBot(before, "")

	newSeq, err := s.PublishLog.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   alice.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

	var tmsgs = [][]byte{
		[]byte(`[1,2,3,4,5]`),
		[]byte(`{"some": 1, "msg": "here"}`),
		[]byte(`{"hello": true}`),
		// []byte(`hello, world`), // invalid json
		[]byte(`"plainStringLikeABlob"`),
		[]byte(`{"hello": false}`),
		[]byte(`{"hello": true}`),
	}

	for i, msg := range tmsgs {
		sbox, err := private.Box(msg, alice, s.KeyPair.Id)
		r.NoError(err, "failed to create ciphertext %d", i)

		newSeq, err := s.PublishLog.Append(sbox)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(newSeq)
	}

	<-ts.doneJS

	uf, ok := s.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(alice.StoredAddr())
	r.NoError(err)

	seqMsg, err := aliceLog.Get(margaret.BaseSeq(1))
	r.NoError(err)
	msg, err := s.RootLog.Get(seqMsg.(margaret.BaseSeq))
	r.NoError(err)
	storedMsg, ok := msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.Equal(storedMsg.Seq(), margaret.BaseSeq(2).Seq())

	ts.wait()
}

func TestPrivMsgsFromJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	bob := ts.gobot

	const n = 16
	alice := ts.startJSBot(`let recps = [ testBob, alice.id ]
	function mkMsg(msg) {
		return function(cb) {
			sbot.private.publish(msg, recps, cb)
		}
	}
	n = 16
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", "i":i}))
	}

    // be done when the other party is done
    sbot.on('rpc:connect', rpc => rpc.on('closed', exit))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n, results.length, "message count")
		run() // triggers connect and after block
	})
`, ``)

	newSeq, err := bob.PublishLog.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   alice.Ref(),
		"following": true,
	})
	r.NoError(err, "failed to publish contact message")
	r.NotNil(newSeq)

	<-ts.doneJS

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(alice.StoredAddr())
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(n-1), seq)

	// var lastMsg string
	for i := 0; i < n; i++ {
		// only one feed in log - directly the rootlog sequences
		seqMsg, err := aliceLog.Get(margaret.BaseSeq(i))
		r.NoError(err)
		r.Equal(seqMsg, margaret.BaseSeq(1+i))

		msg, err := bob.RootLog.Get(seqMsg.(margaret.BaseSeq))
		r.NoError(err)
		absMsg, ok := msg.(refs.Message)
		r.True(ok, "wrong type of message: %T", msg)
		r.Equal(absMsg.Seq(), margaret.BaseSeq(i+1).Seq())

		if i == 0 {
			continue // skip contact
		}
		type testWrap struct {
			Author  refs.FeedRef
			Content string
		}
		var m testWrap
		err = json.Unmarshal(absMsg.ValueContentJSON(), &m)
		// t.Logf("msg:%d:%s", i, string(storedMsg.Raw_))
		r.NoError(err)
		r.True(alice.Equal(&m.Author), "wrong author")
		r.True(strings.HasSuffix(m.Content, ".box"), "test")

		data, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(m.Content, ".box"))
		r.NoError(err)

		clearMsg, err := private.Unbox(bob.KeyPair, data)
		r.NoError(err, "should decrypt the msg %d!", i)

		type testMsg struct {
			Tipe string `json:"type"`
			I    int    `json:"i"`
		}
		var clearObj testMsg
		err = json.Unmarshal(clearMsg, &clearObj)
		r.NoError(err, "should json decode msg %d!", i)
		r.Equal(16-i, clearObj.I, "wrong count on msg %d", i)
		r.Equal("test", clearObj.Tipe)
	}

	ts.wait()
}
