package sbot

import (
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/repo"
)

func makeTestBot(t *testing.T) (*Sbot, []Option) {
	r := require.New(t)

	testPath := filepath.Join("testrun", t.Name())

	appKey := make([]byte, 32)
	rand.Read(appKey)
	hmacKey := make([]byte, 32)
	rand.Read(hmacKey)

	mainLog := testutils.NewRelativeTimeLogger(nil)

	// some extra keypairs for multi-feed fun
	tRepo := repo.New(testPath)
	_, err := repo.NewKeyPair(tRepo, "one", ssb.RefAlgoFeedSSB1)
	r.NoError(err)
	_, err = repo.NewKeyPair(tRepo, "two", ssb.RefAlgoFeedGabby)
	r.NoError(err)

	botOptions := []Option{
		WithAppKey(appKey),
		WithHMACSigning(hmacKey),
		WithInfo(log.With(mainLog, "unit", "theBot")),
		WithRepoPath(testPath),
		DisableNetworkNode(),
	}
	theBot, err := New(botOptions...)
	r.NoError(err)
	return theBot, botOptions
}

func TestFSCK(t *testing.T) {
	testPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(testPath)

	t.Run("correct", testFSCKcorrect)
	t.Run("double", testFSCKdouble)
	t.Run("multipleFeeds", testFSCKmultipleFeeds)
	// t.Run("rerpo", testFSCKrerpo)
}

func testFSCKcorrect(t *testing.T) {
	r := require.New(t)
	theBot, _ := makeTestBot(t)

	const n = 32
	for i := n; i > 0; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	err := theBot.FSCK()
	r.NoError(err)

	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err)

	// cleanup
	theBot.Shutdown()
	r.NoError(theBot.Close())
}

func testFSCKdouble(t *testing.T) {
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.TODO())
	theBot, _ := makeTestBot(t)

	// more valid messages
	const n = 32
	for i := n; i > 0; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	// now do some nasty magic, double the log by appending it to itself again
	// TODO: refactor to only have Add() on the bot, not the internal rootlog
	// Add() should do the append logic
	src, err := theBot.RootLog.Query(margaret.Limit(n))
	r.NoError(err)

	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}

		seq, err := theBot.RootLog.Append(v)
		r.NoError(err)
		t.Log("doubled:", seq.Seq())
	}

	// check duplication
	seqv, err := theBot.RootLog.Seq().Value()
	r.NoError(err)
	seq := seqv.(margaret.Seq)
	r.EqualValues(seq.Seq()+1, n*2)

	// more valid messages
	for i := 64; i > 32; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	// check we get the expected errors
	err = theBot.FSCK(FSCKWithMode(FSCKModeLength))
	r.Error(err)

	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.Error(err)
	constErrs, ok := err.(ErrConsistencyProblems)
	r.True(ok, "wrong error type. got %T", err)
	r.Len(constErrs.Errors, 1)
	r.Contains(constErrs.Errors[0].Error(), "consistency error: message sequence missmatch")

	// try to repair it
	err = theBot.HealRepo(constErrs)
	r.NoError(err)

	// errors are gone
	err = theBot.FSCK(FSCKWithMode(FSCKModeLength))
	r.NoError(err, "after heal (len)")

	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err, "after heal (seq)")

	// cleanup
	theBot.Shutdown()
	cancel()
	r.NoError(theBot.Close())
}

// TODO: copy a corrupted subset of the feed to a fresh rootlog, reindex and see the error

func testFSCKmultipleFeeds(t *testing.T) {
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.TODO())
	theBot, _ := makeTestBot(t)

	// some "correct" messages
	const n = 32
	for i := n; i > 0; i-- {
		_, err := theBot.PublishLog.Publish(i)
		r.NoError(err)
	}

	// create some messages
	intros := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"one", map[string]interface{}{"hello": 123}},
		{"one", map[string]interface{}{"world": 456}},
		{"two", map[string]interface{}{"test": 123}},
		{"two", map[string]interface{}{"test": 456}},
		{"two", map[string]interface{}{"test": 789}},
	}
	for idx, intro := range intros {
		ref, err := theBot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
	}

	// copy the messages from one and two (leaving "main" intact)
	src, err := theBot.RootLog.Query(
		margaret.Gt(margaret.BaseSeq(n-1)),
		margaret.Limit(5))
	r.NoError(err)
	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}

		msg, ok := v.(refs.Message)
		r.True(ok)

		seq, err := theBot.RootLog.Append(v)
		r.NoError(err)
		t.Log("doubled:", msg.Author().ShortRef(), seq.Seq())
	}

	err = theBot.FSCK(FSCKWithMode(FSCKModeLength))
	r.Error(err)

	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.Error(err)
	constErrs, ok := err.(ErrConsistencyProblems)
	r.True(ok, "wrong error type. got %T", err)
	r.Len(constErrs.Errors, 2)

	// try to repair it
	err = theBot.HealRepo(constErrs)
	r.NoError(err)

	// errors are gone
	err = theBot.FSCK(FSCKWithMode(FSCKModeLength))
	r.NoError(err, "after heal (len)")

	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err, "after heal (seq)")

	// cleanup
	theBot.Shutdown()
	cancel()
	r.NoError(theBot.Close())
}

// to use this, put the repo in
func testFSCKrepro(t *testing.T) {
	r := require.New(t)
	r.EqualValues(t.Name,"foo")

	const buggedRepo = "testdata/example-repo-with-a-bug"
	tRepoPath := filepath.Join("testrun", t.Name())
	t.Log(tRepoPath)
	os.MkdirAll("testrun/TestFSCK", 0700)
	//tRepo := repo.New(tRepoPath)

	out, err := exec.Command("cp", "-v", "-r",buggedRepo, tRepoPath).CombinedOutput()
	r.NoError(err, "got: %s", string(out))

	theBot, _ := makeTestBot(t)

	seqV, err := theBot.RootLog.Seq().Value()
	r.NoError(err)
	latestSeq := seqV.(margaret.Seq)
	r.EqualValues(latestSeq.Seq(), 6699)

	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.Error(err)
	constErrs, ok := err.(ErrConsistencyProblems)
	r.True(ok, "wrong error type. got %T", err)

	// repair it
	err = theBot.HealRepo(constErrs)
	r.NoError(err)

	// error is gone
	err = theBot.FSCK(FSCKWithMode(FSCKModeSequences))
	r.NoError(err)

	// cleanup
	theBot.Shutdown()
	r.NoError(theBot.Close())
}
