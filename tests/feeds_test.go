// SPDX-License-Identifier: MIT

package tests

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/sbot"
)

func TestFeedFromJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)
	const n = 23

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

	// just for keygen, needed later
	claire := ts.startJSBotWithName("claire", `exit()`, "")

	alice := ts.startJSBot(fmt.Sprintf(`
	let claireRef = %q
	function mkMsg(msg) {
		return function(cb) {
			sbot.publish(msg, cb)
		}
	}
	n = 23
	let msgs = []
	for (var i = n; i>0; i--) {
		msgs.push(mkMsg({type:"test", text:"foo", i:i}))
	}
	msgs.push(mkMsg({type: 'contact', contact: claireRef, following: true}))

	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))

	parallel(msgs, function(err, results) {
		t.error(err, "parallel of publish")
		t.equal(n+1, results.length, "message count")
		run() // triggers connect and after block
	})
`, claire.Ref()), ``)

	bob.Replicate(alice)

	<-ts.doneJS

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(alice.StoredAddr())
	r.NoError(err)
	seq, err := aliceLog.Seq().Value()
	r.NoError(err)
	r.Equal(margaret.BaseSeq(n), seq)

	var lastMsg string
	for i := 0; i < n; i++ { // don't check the contact:following message from A to C
		msg, err := mutil.Indirect(bob.RootLog, aliceLog).Get(margaret.BaseSeq(i))
		r.NoError(err)
		storedMsg, ok := msg.(refs.Message)
		r.True(ok, "wrong type of message: %T", msg)
		r.Equal(storedMsg.Seq(), margaret.BaseSeq(i+1).Seq())

		type testWrap struct {
			Author  refs.FeedRef
			Content struct {
				Type, Text string
				I          int
			}
		}
		var m testWrap
		err = json.Unmarshal(storedMsg.ValueContentJSON(), &m)
		r.NoError(err)
		r.True(alice.Equal(&m.Author), "wrong author")
		r.Equal(m.Content.Type, "test")
		r.Equal(m.Content.Text, "foo")
		r.Equal(m.Content.I, n-i, "wrong I on msg: %d", i)
		if i == n-1 {
			lastMsg = storedMsg.Key().Ref()
		}
	}

	before := fmt.Sprintf(`
aliceRef = %q // global - pubKey of the first alice

t.comment('shouldnt have alices feed:' + aliceRef)

sbot.on('rpc:connect', (rpc) => {
  rpc.on('closed', () => { 
    t.comment('now should have feed:' + aliceRef)
    pull(
      sbot.createUserStream({id:aliceRef, reverse:true, limit: 2}),
      pull.collect(function(err, msgs) {
        t.error(err, 'query worked')
		t.equal(2, msgs.length, 'got all the messages')
		// skip the contact message
		t.equal('contact', msgs[0].value.content.type, 'latest sequence')
        t.equal(%q, msgs[1].key, 'latest keys match')
        t.equal(23, msgs[1].value.sequence, 'latest sequence')
        exit()
      })
    )
  })
})

sbot.publish({type: 'contact', contact: aliceRef, following: true}, function(err, msg) {
  t.error(err, 'follow:' + aliceRef)

sbot.friends.get({src: alice.id, dest: aliceRef}, function(err, val) {
  t.error(err, 'friends.get of new contact')
  t.equals(val[alice.id], true, 'is following')

pull(
  sbot.createUserStream({id:aliceRef}),
  pull.collect(function(err, vals){
    t.error(err)
    t.equal(0, vals.length)
    run() // connect to go-sbot
  })
)

}) // friends.get

}) // publish`, alice.Ref(), lastMsg)
	claire = ts.startJSBotWithName("claire", before, "")

	t.Logf("started claire: %s", claire.Ref())
	bob.Replicate(claire)

	ts.wait()
}

func TestFeedFromGo(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	before := `fromKey = testBob
	sbot.on('rpc:connect', (rpc) => {
		rpc.on('closed', () => {
			t.comment('now should have feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey, reverse:true, limit: 3}),
				pull.collect(function(err, msgs) {
					t.error(err, 'query worked')
					t.equal(msgs.length, 3, 'got all the messages')
					// t.comment(JSON.stringify(msgs[0]))
					t.equal(msgs[0].value.sequence, 3, 'sequence:0')
					t.equal(msgs[1].value.sequence, 2, 'sequence:1')
					t.equal(msgs[2].value.sequence, 1, 'sequence:2')
					exit()
				})
			)
		})
	})

	sbot.publish({type: 'contact', contact: fromKey, following: true}, function(err, msg) {
		t.error(err, 'follow:' + fromKey)

		sbot.friends.get({src: alice.id, dest: fromKey}, function(err, val) {
			t.error(err, 'friends.get of new contact')
			t.equals(val[alice.id], true, 'is following')

			t.comment('shouldnt have bobs feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey}),
				pull.collect(function(err, vals){
					t.error(err)
					t.equal(0, vals.length)
					sbot.publish({type: 'about', about: fromKey, name: 'test bob'}, function(err, msg) {
						t.error(err, 'about:' + msg.key)
						setTimeout(run, 1000)
					})
				})
			)

}) // friends.get

}) // publish`

	alice := ts.startJSBot(before, "")
	s.Replicate(alice)
	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": s.KeyPair.Id.Ref(),
			"name":  "test user",
		},
		map[string]interface{}{
			"type": "text",
			"text": `# hello world!`,
		},
		map[string]interface{}{
			"type":  "about",
			"about": alice.Ref(),
			"name":  "test alice",
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := s.PublishLog.Append(msg)
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
	r.EqualValues(storedMsg.Seq(), 2)

	s.Network.GetConnTracker().CloseAll()
	ts.wait()

	t.Log("restarting for integrity check")
	ts.startGoBot()
	s = ts.gobot
	err = s.FSCK(sbot.FSCKWithMode(sbot.FSCKModeSequences))
	r.NoError(err)

	ml, ok := s.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err = ml.Get(alice.StoredAddr())
	r.NoError(err)
	aseq, err := aliceLog.Seq().Value()
	r.NoError(err)
	seqMsg, err = aliceLog.Get(aseq.(margaret.BaseSeq))
	r.NoError(err)
	msg, err = s.RootLog.Get(seqMsg.(margaret.BaseSeq))
	r.NoError(err)
	storedMsg, ok = msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(storedMsg.Seq(), 2)

	bobLog, err := ml.Get(s.KeyPair.Id.StoredAddr())
	r.NoError(err)
	bseq, err := bobLog.Seq().Value()
	r.NoError(err)
	seqMsg, err = bobLog.Get(bseq.(margaret.BaseSeq))
	r.NoError(err)
	msg, err = s.RootLog.Get(seqMsg.(margaret.BaseSeq))
	r.NoError(err)
	storedMsg, ok = msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.EqualValues(storedMsg.Seq(), 3)

	err = s.FSCK(sbot.FSCKWithMode(sbot.FSCKModeSequences))
	r.NoError(err)

	ts.wait()
}

// We need more complete tests that cover joining and leaving peers to make sure we don't leak querys, rpc streams or other goroutiens
func XTestFeedFromGoLive(t *testing.T) {
	defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	before := `fromKey = testBob
	pull(
		sbot.createHistoryStream({id:fromKey, live:true}),
		pull.drain(function(msg) {
			t.comment("got message!"+ msg.value.sequence)
			// t.comment(JSON.stringify(msg).key)
			if (msg.value.sequence == 5) {
				
				t.comment("waited")
				exit()
			}
		})
	)
	sbot.publish({type: 'contact', contact: fromKey, following: true}, function(err, msg) {
		t.error(err, 'follow:' + fromKey)

		sbot.friends.get({src: alice.id, dest: fromKey}, function(err, val) {
			t.error(err, 'friends.get of new contact')
			t.equals(val[alice.id], true, 'is following')

			t.comment('shouldnt have bobs feed:' + fromKey)
			pull(
				sbot.createUserStream({id:fromKey}),
				pull.collect(function(err, vals){
					t.error(err)
					t.equal(0, vals.length)
					sbot.publish({type: 'about', about: fromKey, name: 'test bob'}, function(err, msg) {
						t.error(err, 'about:' + msg.key)
						setTimeout(run, 1000) // give go bot a moment to publish
					})
				})
			)

}) // friends.get

}) // publish`

	alice := ts.startJSBot(before, "")

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": s.KeyPair.Id.Ref(),
			"name":  "test user",
		},
		map[string]interface{}{
			"type":      "contact",
			"contact":   alice.Ref(),
			"following": true,
		},
		map[string]interface{}{
			"type": "text",
			"text": `# hello world!`,
		},
		map[string]interface{}{
			"type":  "about",
			"about": alice.Ref(),
			"name":  "test alice",
		},
	}
	for i, msg := range tmsgs {
		ref, err := s.PublishLog.Publish(msg)
		r.NoError(err, "failed to publish test message %d", i)
		r.NotNil(ref)
	}

	time.Sleep(2 * time.Second)
	ref, err := s.PublishLog.Publish(map[string]interface{}{
		"type": "test",
		"live": true,
	})
	r.NoError(err)
	r.NotNil(ref)
	fmt.Println("send late msg")

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
