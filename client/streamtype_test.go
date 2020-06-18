// SPDX-License-Identifier: MIT

package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
)

func TestReadStreamAsInterfaceMessage(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)

	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		sbot.WithListenAddr(":0"))
	r.NoError(err, "sbot srv init failed")

	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(context.TODO())
		if err != nil {
			srvErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(srvErrc)
	}()

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()
	r.NotNil(srvAddr, "listener not ready")

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	// no messages yet
	seqv, err := srv.RootLog.Seq().Value()
	r.NoError(err, "failed to get root log sequence")
	r.Equal(margaret.SeqEmpty, seqv)

	type testMsg struct {
		Foo string
		Bar int
	}
	var refs []string
	for i := 0; i < 10; i++ {

		msg := testMsg{"hello", 23}
		ref, err := c.Publish(msg)
		r.NoError(err, "failed to call publish")
		r.NotNil(ref)

		// get stored message from the log
		seqv, err = srv.RootLog.Seq().Value()
		r.NoError(err, "failed to get root log sequence")
		wantSeq := margaret.BaseSeq(i)
		a.Equal(wantSeq, seqv)
		msgv, err := srv.RootLog.Get(wantSeq)
		r.NoError(err)
		newMsg, ok := msgv.(refs.Message)
		r.True(ok)
		r.Equal(newMsg.Key(), ref)
		refs = append(refs, ref.Ref())

		opts := message.CreateLogArgs{}
		opts.Keys = true
		opts.Limit = 1
		opts.Seq = int64(i)
		opts.MarshalType = ssb.KeyValueRaw{}
		src, err := c.CreateLogStream(opts)
		r.NoError(err)

		streamV, err := src.Next(context.TODO())
		r.NoError(err, "failed to next msg:%d", i)
		streamMsg, ok := streamV.(refs.Message)
		r.True(ok, "acutal type: %T", streamV)
		a.Equal(newMsg.Author().Ref(), streamMsg.Author().Ref())

		a.EqualValues(newMsg.Seq(), streamMsg.Seq())

		v, err := src.Next(context.TODO())
		a.Nil(v)
		a.Equal(luigi.EOS{}, errors.Cause(err))
	}

	opts := message.CreateLogArgs{}
	opts.Keys = true
	opts.Limit = 10
	opts.MarshalType = ssb.KeyValueRaw{}
	src, err := c.CreateLogStream(opts)
	r.NoError(err)

	for i := 0; i < 10; i++ {
		streamV, err := src.Next(context.TODO())
		r.NoError(err, "failed to next msg:%d", i)
		msg, ok := streamV.(refs.Message)
		r.True(ok, "acutal type: %T", streamV)
		a.Equal(refs[i], msg.Key().Ref())
	}

	v, err := src.Next(context.TODO())
	a.Nil(v)
	a.Equal(luigi.EOS{}, errors.Cause(err))

	a.NoError(c.Close())

	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
