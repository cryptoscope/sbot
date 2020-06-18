package sbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/sync/errgroup"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/indexes"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/repo"
)

func TestNullContentRequest(t *testing.T) {
	// defer leakcheck.Check(t)

	r := require.New(t)
	a := assert.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(tRepoPath)

	// make three new keypairs with nicknames
	n2kp := make(map[string]*ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", ssb.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 2)

	// make the bot
	logger := testutils.NewRelativeTimeLogger(nil)

	mainbot, err := New(
		WithInfo(logger),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		LateOption(MountSimpleIndex("get", indexes.OpenGet)),
		DisableNetworkNode(),
	)
	r.NoError(err)

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", ssb.NewContactFollow(kpBert.Id)},
		{"bert", ssb.NewContactFollow(kpArny.Id)},
		{"arny", map[string]interface{}{"test": 123}},
		{"bert", map[string]interface{}{"world": 456}},
		{"bert", map[string]interface{}{"spew": true, "delete": "me"}},
		{"bert", map[string]interface{}{"more": true, "text": "previous was really meh..."}},
		{"bert", map[string]interface{}{"more": true, "text": "wish i could make that go away..."}},
		{"arny", map[string]interface{}{"text": "i think you should be able to.."}},
	}

	var allMessages []refs.MessageRef
	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
		msg, err := mainbot.Get(*ref)
		r.NoError(err)
		r.NotNil(msg)

		r.True(msg.Author().Equal(n2kp[intro.as].Id))

		allMessages = append(allMessages, *ref)
	}

	// assert helper
	checkLogSeq := func(l margaret.Log, seq int) {
		v, err := l.Seq().Value()
		r.NoError(err)
		r.EqualValues(seq, v.(margaret.Seq).Seq())
	}

	checkUserLogSeq := func(bot *Sbot, name string, seq int) {
		kp, has := n2kp[name]
		r.True(has, "%s not in map", name)

		uf, ok := bot.GetMultiLog("userFeeds")
		r.True(ok, "userFeeds mlog not present")

		l, err := uf.Get(kp.Id.StoredAddr())
		r.NoError(err)

		checkLogSeq(l, seq)
	}

	checkLogSeq(mainbot.RootLog, len(intros)-1) // got all the messages

	// check before drop
	checkUserLogSeq(mainbot, "arny", 2)
	checkUserLogSeq(mainbot, "bert", 4)

	// get hash of message to drop
	uf, ok := mainbot.GetMultiLog("userFeeds")
	r.True(ok, "userFeeds mlog not present")

	// try to request on arnies feed fails because the formt doesn't support it
	arniesLog, err := uf.Get(kpArny.Id.StoredAddr())
	r.NoError(err)

	arniesLog = mutil.Indirect(mainbot.RootLog, arniesLog)
	dcr := ssb.NewDropContentRequest(1, allMessages[0])
	r.False(dcr.Valid(arniesLog))

	// bert is in gg format so it works
	bertLog, err := uf.Get(kpBert.Id.StoredAddr())
	r.NoError(err)

	bertLog = mutil.Indirect(mainbot.RootLog, bertLog)
	msgv, err := bertLog.Get(margaret.BaseSeq(2)) // 0-indexed
	r.NoError(err)
	msg, ok := msgv.(refs.Message)
	r.True(ok, "not a msg! %T", msgv)

	r.True(msg.Author().Equal(kpBert.Id), "wrong author")
	r.True(bytes.Contains(msg.ContentBytes(), []byte(`"delete":`)), "wrong message")

	type tcase struct {
		seq    uint
		msgkey refs.MessageRef

		okay bool
	}
	fakeRef := refs.MessageRef{}
	cases := []tcase{
		{1, fakeRef, false},
		{1, allMessages[0], false},
		{1, allMessages[1], true},
		{0, allMessages[1], false},

		{3, allMessages[4], true},
	}

	for ic, c := range cases {
		tmsg := ssb.NewDropContentRequest(c.seq, c.msgkey)
		_, err = json.Marshal(tmsg)
		r.NoError(err)
		a.Equal(c.okay, tmsg.Valid(bertLog), "%d: failed", ic)
	}

	dropContent := ssb.NewDropContentRequest(3, *msg.Key())
	v, err := json.Marshal(dropContent)
	r.NoError(err)
	t.Log(string(v))

	msg, err = mainbot.Get(*msg.Key())
	r.NoError(err)
	origContent := msg.ContentBytes()
	a.NotNil(origContent)

	del, err := mainbot.PublishAs("bert", dropContent)
	r.NoError(err)
	r.NotNil(del)
	t.Log("first, valid dcr request:", del.Ref())
	logger.Log("msg", "req published")

	time.Sleep(1 * time.Second)
	logger.Log("msg", "waited")

	// aaand it's gone
	msg, err = mainbot.Get(*msg.Key())
	r.NoError(err)
	nulledContent := msg.ContentBytes()
	a.Nil(nulledContent, "content not nil")
	a.NotEqual(origContent, nulledContent, "content still the same!")
	logger.Log("msg", "checked")
	a.NotNil(msg.Author(), "author is still there")
	a.NotNil(msg.Seq(), "sequence is still there")

	// can't delete a delete
	cantDropThis := ssb.NewDropContentRequest(6, *del)
	a.False(cantDropThis.Valid(bertLog), "can't delete a delete")

	// still try
	del2, err := mainbot.PublishAs("bert", cantDropThis)
	r.NoError(err)
	r.NotNil(del2)
	t.Log("invalid dcr:", del2.Ref())

	// not gone
	msg, err = mainbot.Get(*del2)
	r.NoError(err)
	a.NotNil(msg.ContentBytes())

	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}

func TestNullContentAndSync(t *testing.T) {
	// // defer leakcheck.Check(t)
	r := require.New(t)
	// a := assert.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)

	os.RemoveAll(filepath.Join("testrun", t.Name()))
	tRepoPath := filepath.Join("testrun", t.Name(), "botOne")
	tRepo := repo.New(tRepoPath)

	logger := testutils.NewRelativeTimeLogger(nil)

	ctx, cancel := context.WithCancel(context.TODO())
	botgroup, ctx := errgroup.WithContext(ctx)

	bs := newBotServer(ctx, logger)

	// make three new keypairs with nicknames
	n2kp := make(map[string]*ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", ssb.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 2)

	// assert helpers
	checkUserLogSeq := func(bot *Sbot, name string, want int) {
		kp, has := n2kp[name]
		r.True(has, "%s not in map", name)

		uf, ok := bot.GetMultiLog("userFeeds")
		r.True(ok, "userFeeds mlog not present")

		l, err := uf.Get(kp.Id.StoredAddr())
		r.NoError(err)

		v, err := l.Seq().Value()
		r.NoError(err)
		r.EqualValues(want, v.(margaret.Seq).Seq(), "userFeed of %s has unexpected sequence", name)
	}

	checkMessageNulled := func(bot *Sbot, name string, seq uint, isNulled bool) {
		kp, has := n2kp[name]
		r.True(has, "%s not in map", name)

		uf, ok := bot.GetMultiLog("userFeeds")
		r.True(ok, "userFeeds mlog not present")

		userLog, err := uf.Get(kp.Id.StoredAddr())
		r.NoError(err)

		msgv, err := mutil.Indirect(bot.RootLog, userLog).Get(margaret.BaseSeq(seq - 1)) // 0-indexed
		r.NoError(err)

		msg, ok := msgv.(refs.Message)
		r.True(ok, "not a msg! %T", msgv)

		if isNulled {
			c := msg.ContentBytes()
			if c != nil {
				t.Log("Content Dump:\n" + spew.Sdump(c))
			}
			r.Nil(c, "%s:%d message not nulled", name, seq)
		} else {
			r.NotNil(msg.ContentBytes(), "%s:%d message IS nulled", name, seq)
		}
	}

	// make the first bot

	mainbot, err := New(
		WithInfo(log.With(logger, "bot", "1")),
		WithRepoPath(tRepoPath),
		WithHMACSigning(hk),
		WithPromisc(true),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(mainbot))

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", ssb.NewContactFollow(kpBert.Id)},
		{"bert", ssb.NewContactFollow(kpArny.Id)},
		{"arny", map[string]interface{}{"test": 123}},
		{"bert", map[string]interface{}{"world": 456}},
		{"bert", map[string]interface{}{"spew": true, "delete": "me"}},
		{"bert", map[string]interface{}{"more": true, "text": "previous was really meh..."}},
		{"bert", map[string]interface{}{"more": true, "text": "wish i could make that go away..."}},
		{"arny", map[string]interface{}{"text": "i think you should be able to.."}},
		{"bert", map[string]interface{}{"more": true, "text": "hope this sticks...?"}},
		{"arny", map[string]interface{}{"text": "yea, i feel you.."}},
	}
	for idx, intro := range intros {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
	}

	// all the messages
	checkUserLogSeq(mainbot, "arny", 3)
	checkUserLogSeq(mainbot, "bert", 5)

	// can't null self (because it's in old fomrat)
	r.Equal(mainbot.KeyPair.Id.Algo, ssb.RefAlgoFeedSSB1, "wrong feed format (upgraded default?)")
	err = mainbot.NullContent(kpArny.Id, 2)
	r.Error(err, "should not work")
	r.EqualError(err, ssb.ErrUnuspportedFormat.Error())
	checkMessageNulled(mainbot, "arny", 2, false)

	// null some content manually
	err = mainbot.NullContent(kpBert.Id, 3)
	r.NoError(err, "failed to drop delete: me")
	checkMessageNulled(mainbot, "bert", 3, true)

	err = mainbot.NullContent(kpBert.Id, 4)
	r.NoError(err, "failed to drop delete: me")
	checkMessageNulled(mainbot, "bert", 4, true)

	// make the 2nd bot
	otherBot, err := New(
		WithInfo(log.With(logger, "bot", "2")),
		WithRepoPath(filepath.Join("testrun", t.Name(), "botTwo")),
		WithHMACSigning(hk),
		WithPromisc(true),
		WithListenAddr(":0"),
	)
	r.NoError(err)
	botgroup.Go(bs.Serve(otherBot))

	// conect, should still get the full feeds
	mainbot.Replicate(otherBot.KeyPair.Id)
	otherBot.Replicate(mainbot.KeyPair.Id)
	otherBot.Replicate(kpArny.Id)
	otherBot.Replicate(kpBert.Id)
	err = mainbot.Network.Connect(ctx, otherBot.Network.GetListenAddr())
	r.NoError(err)

	time.Sleep(2 * time.Second)

	checkUserLogSeq(otherBot, "arny", 3)
	checkUserLogSeq(otherBot, "bert", 5)

	checkMessageNulled(otherBot, "bert", 3, true)
	checkMessageNulled(otherBot, "bert", 4, true)

	// print all the messages
	printSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			logger.Log("err", err)
			return err
		}

		sw, ok := v.(margaret.SeqWrapper)
		r.True(ok, "not a SW! %T", v)

		msg, ok := sw.Value().(refs.Message)
		r.True(ok, "not a msg! %T", sw)

		t.Log(sw.Seq().Seq(), string(msg.ContentBytes()))
		return err
	})
	src, err := otherBot.RootLog.Query(margaret.SeqWrap(true))
	r.NoError(err)
	luigi.Pump(context.Background(), printSink, src)

	mainbot.Shutdown()
	otherBot.Shutdown()

	cancel()
	r.NoError(mainbot.Close())
	r.NoError(otherBot.Close())

	r.NoError(botgroup.Wait())
}
