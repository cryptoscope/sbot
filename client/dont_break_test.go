package client_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	"go.cryptoscope.co/ssb/internal/testutils"
	"go.cryptoscope.co/ssb/message"
	"go.cryptoscope.co/ssb/sbot"
)

// this is only a basic test.
// ideally we would want to feed invalid messages to the verification
// and check that it doesn't drop the connection just because an error on one stream
func TestAskForSomethingWeird(t *testing.T) {
	r, a := require.New(t), assert.New(t)

	srvRepo := filepath.Join("testrun", t.Name(), "serv")
	os.RemoveAll(srvRepo)
	srvLog := testutils.NewRelativeTimeLogger(nil)

	srv, err := sbot.New(
		sbot.WithInfo(srvLog),
		sbot.WithRepoPath(srvRepo),
		// sbot.DisableNetworkNode(), skips muxrpc handler
		sbot.WithListenAddr(":0"),
	)
	r.NoError(err, "sbot srv init failed")

	ctx := context.TODO()
	ctx, cancel := context.WithCancel(ctx)
	var srvErrc = make(chan error, 1)
	go func() {
		err := srv.Network.Serve(ctx)
		if err != nil && err != context.Canceled {
			srvErrc <- errors.Wrap(err, "ali serve exited")
		}
		close(srvErrc)
	}()

	kp, err := ssb.LoadKeyPair(filepath.Join(srvRepo, "secret"))
	r.NoError(err, "failed to load servers keypair")
	srvAddr := srv.Network.GetListenAddr()

	c, err := client.NewTCP(kp, srvAddr)
	r.NoError(err, "failed to make client connection")
	// end test boilerplate

	ref, err := c.Whoami()
	r.NoError(err, "failed to call whoami")
	r.NotNil(ref)
	a.Equal(srv.KeyPair.Id.Ref(), ref.Ref())

	// make sure we can publish
	var msgs []*refs.MessageRef
	const msgCount = 15
	for i := 0; i < msgCount; i++ {
		ref, err := c.Publish(struct{ I int }{i})
		r.NoError(err)
		r.NotNil(ref)
		msgs = append(msgs, ref)
	}

	// and stream those messages back
	var o message.CreateHistArgs
	o.ID = srv.KeyPair.Id
	o.Keys = true
	o.MarshalType = ssb.KeyValueRaw{}
	src, err := c.CreateHistoryStream(o)
	r.NoError(err)
	r.NotNil(src)

	i := 0
	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			r.NoError(err)
		}
		r.NotNil(v)

		if i == 5 {
			var o message.CreateHistArgs
			o.ID = &refs.FeedRef{
				Algo: "wrong",
				ID:   bytes.Repeat([]byte("nope"), 8),
			}
			o.Keys = true
			o.MarshalType = ssb.KeyValueRaw{}
			// starting the call works (although our lib could check that the ref is wrong, too)
			src, err := c.CreateHistoryStream(o)
			a.NoError(err)
			a.NotNil(src)
			v, err := src.Next(ctx)
			a.Nil(v, "did not expect value from source: %T", v)
			a.Error(err)
			ce := errors.Cause(err)
			callErr, ok := ce.(*muxrpc.CallError)
			r.True(ok, "not a call err: %T", ce)
			t.Log(callErr)
		}

		msg, ok := v.(refs.Message)
		r.True(ok, "%d: wrong type: %T", i, v)

		r.True(msg.Key().Equal(*msgs[i]), "wrong message %d", i)
		i++
	}
	r.Equal(msgCount, i, "did not get all messages")

	a.NoError(c.Close())
	cancel()
	srv.Shutdown()
	r.NoError(srv.Close())
	r.NoError(<-srvErrc)
}
