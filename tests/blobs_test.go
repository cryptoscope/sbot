// SPDX-License-Identifier: MIT

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"go.cryptoscope.co/ssb/internal/mutil"
	refs "go.mindeco.de/ssb-refs"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
)

func TestBlobToJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	alice := ts.startJSBot(`run()`,
		`setTimeout(() => {
			sbot.blobs.want("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256",function(err, has) {
				t.true(has, "got blob")
				t.error(err, "no err")
				exit()
			})
		}, 1000)`)
	s.Replicate(alice)

	ref, err := s.BlobStore.Put(strings.NewReader("bl0000p123123"))
	r.NoError(err)
	r.Equal("&rCJbx8pzYys3zFkmXyYG6JtKZO9/LX51AMME12+WvCY=.sha256", ref.Ref())

	ts.wait()

	// TODO: check wantManager for this connection is stopped when the jsbot exited

}

func TestBlobFromJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	const fooBarRef = "&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256"
	testRef, err := ssb.ParseBlobRef(fooBarRef) // foobar
	r.NoError(err)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	alice := ts.startJSBot(
		`
/* pinned to 1.1.14
		pull(sbot.blobs.changes(), pull.drain(function(v) {
			// migitation against blobs blocking
			// https://github.com/ssbc/ssb-blobs/pulls/17
			t.equal(v, '&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256')
			run()
		}))
*/
		pull(
			pull.values([Buffer.from("foobar")]),
			sbot.blobs.add(function(err, id) {
				t.error(err, "added err")
				t.equal(id, '&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256', "blob id")
				run()
			})
		)`,
		`sbot.blobs.has(
			"&w6uP8Tcg6K2QR905Rms8iXTlksL6OD1KOWBxTK7wxPI=.sha256",
			function(err, has) {
				t.true(has, "should have blob")
				t.error(err, "has err")
				setTimeout(exit, 1500)
			})`,
	)
	s.Replicate(alice)

	got := make(chan struct{})
	s.BlobStore.Changes().Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		notif := v.(ssb.BlobStoreNotification)
		if ssb.BlobStoreOp("put") == notif.Op && fooBarRef == notif.Ref.Ref() {
			close(got)
		} else {
			fmt.Println("warning: wrong blob notify!", notif)
		}
		return err
	}))

	err = s.WantManager.Want(testRef)
	r.NoError(err, ".Want() should not error")

	<-got

	br, err := s.BlobStore.Get(testRef)
	r.NoError(err, "should have blob")

	foobar, err := ioutil.ReadAll(br)
	r.NoError(err, "couldnt read blob")
	r.Equal("foobar", string(foobar))

	ts.wait()

}

func TestBlobWithHop(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)

	ts.startGoBot()
	bob := ts.gobot

	alice := ts.startJSBot(`
	pull(
		pull.once(Buffer.from("whopwhopwhop")),
		sbot.blobs.add(function(err, hash) {
			t.error(err)
			t.comment('got hash:'+hash)
			sbot.blobs.size(hash, (err, sz) => {
				t.error(err)
				t.comment('size'+sz)
				sbot.publish({
					type:'test',
					blob: hash,
				}, (err, msg) => {
					t.error(err)
					t.comment('leaked blob addr in:'+msg.key)
					run()
				})
			})
		})
	  )
	
	// be done when the other party is done
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))
`, ``)
	bob.Replicate(alice)

	aliceDone := ts.doneJS

	uf, ok := bob.GetMultiLog("userFeeds")
	r.True(ok)
	aliceLog, err := uf.Get(alice.StoredAddr())
	r.NoError(err)

	gotMessage := make(chan struct{})
	updateSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		seq, has := v.(margaret.Seq)
		if !has {
			return fmt.Errorf("unexpected type:%T", v)
		}
		if seq.Seq() == 0 {
			close(gotMessage)
		}
		return err
	})
	done := aliceLog.Seq().Register(updateSink)

	<-gotMessage
	done()

	var wantBlob *refs.BlobRef

	msg, err := mutil.Indirect(bob.RootLog, aliceLog).Get(margaret.BaseSeq(0))
	r.NoError(err)
	storedMsg, ok := msg.(refs.Message)
	r.True(ok, "wrong type of message: %T", msg)
	r.Equal(storedMsg.Seq(), margaret.BaseSeq(1).Seq())

	type testWrap struct {
		Author  refs.FeedRef
		Content struct {
			Type string
			Blob *refs.BlobRef
		}
	}
	var m testWrap
	err = json.Unmarshal(storedMsg.ValueContentJSON(), &m)
	r.NoError(err)
	r.True(alice.Equal(&m.Author), "wrong author")
	r.Equal("test", m.Content.Type)
	r.NotNil(m.Content.Blob)

	wantBlob = m.Content.Blob

	// bob.WantManager.Want(wantBlob)

	before := fmt.Sprintf(`wantHash = %q // blob we want from alice

pull(
	sbot.blobs.changes(),
	pull.drain((evt)=> {
		t.comment('blobs:change:' + JSON.stringify(evt))
	})
)

sbot.blobs.want(wantHash, (err, has) => {
	t.error(err, 'want err?')
	exit()
})

run()
`, wantBlob.Ref())

	claire := ts.startJSBot(before, "")

	t.Logf("started claire: %s", claire.Ref())
	bob.Replicate(claire)

	ts.wait()
	<-aliceDone
}

func XTestBlobTooBigWantedByJS(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	zerof, err := os.Open("/dev/zero")
	r.NoError(err)
	defer zerof.Close()

	const smallEnough = blobstore.DefaultMaxSize - 10
	small, err := s.BlobStore.Put(io.LimitReader(zerof, smallEnough))
	r.NoError(err)

	const veryLarge = blobstore.DefaultMaxSize + 10
	big, err := s.BlobStore.Put(io.LimitReader(zerof, veryLarge))
	r.NoError(err)

	t.Log("added too big", big.Ref())
	ts.startJSBot(`timeoutLength = 600000;run()`,
		fmt.Sprintf(`
		let small = %q
		let big = %q
		setTimeout(() => {
			// this just times out, need to find a different way to handle this
			// the wants pipe returns the size and thus the jsbot doesn't request it
			sbot.blobs.want(big, (err, has) => {
				t.false(has, "did got big blob (shouldn't have)")
				t.error(err)
			})
			sbot.blobs.want(small, (err, has) => {
				t.true(has, "got small blob")
				t.error(err, "no err")
				setTimeout(exit, 5000)
			})
		}, 1000)
		`, small.Ref(), big.Ref()))

	ts.wait()
}

func TestBlobTooBigWantedByGo(t *testing.T) {
	// defer leakcheck.Check(t)
	r := require.New(t)

	ts := newRandomSession(t)
	// ts := newSession(t, nil, nil)

	ts.startGoBot()
	s := ts.gobot

	zerof, err := os.Open("/dev/zero")
	r.NoError(err)
	defer zerof.Close()

	jsBot := ts.startJSBot(`
	
	pull(
		pull.once(Buffer.alloc(5*1024*1024-10)),
		sbot.blobs.add(function(err, smallHash) {
			t.error(err)
			t.comment('got hash:'+smallHash)

			pull(
				pull.once(Buffer.alloc(5*1024*1024+10)),
				sbot.blobs.add(function(err, bigHash) {
					t.error(err)
					t.comment('got hash:'+bigHash)
		
					sbot.publish({
						type:"test-data",
						small: smallHash,
						big: bigHash,
					}, (err, key) => {
						t.error(err)
						t.comment('published test-data')
						run()
					})
				})
			)
		})
	)
	sbot.on('rpc:connect', rpc => rpc.on('closed', exit))`, ``)
	s.Replicate(jsBot)

	uf, ok := s.GetMultiLog("userFeeds")
	r.True(ok)

	jsFeedSeqs, err := uf.Get(jsBot.StoredAddr())
	r.NoError(err)
	jsFeed := mutil.Indirect(s.RootLog, jsFeedSeqs)
	tries := 10
	var testData struct {
		Type  string        `json:"test-data"`
		Small *refs.BlobRef `json:"small"`
		Big   *refs.BlobRef `json:"big"`
	}
	for tries > 0 {

		v, err := jsFeed.Get(margaret.BaseSeq(0))
		if err == nil {
			msg, ok := v.(refs.Message)
			r.True(ok, "not a message")

			err = json.Unmarshal(msg.ContentBytes(), &testData)
			r.NoError(err)
			break
		}
		time.Sleep(1 * time.Second)
		tries--
	}
	if tries == 0 {
		t.Fatal("did not get test-data message")
	}
	r.Equal("&SqtVEGDZDsEI53s3k9lHORXAgFjYmRK7pFDcAuYoo2c=.sha256", testData.Small.Ref())
	r.Equal("&BcIXzoawDrJrq7ETyZxLmbL/cJzPnRCu76l5Qlgw1T4=.sha256", testData.Big.Ref())
	s.WantManager.Want(testData.Small)
	s.WantManager.Want(testData.Big)
	time.Sleep(3 * time.Second)

	sz, err := s.BlobStore.Size(testData.Small)
	r.NoError(err)
	r.EqualValues(blobstore.DefaultMaxSize-10, sz)

	sz, err = s.BlobStore.Size(testData.Big)
	r.Error(err)
	r.EqualValues(0, sz)

	ts.wait()
}
