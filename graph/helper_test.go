// SPDX-License-Identifier: MIT

package graph

import (
	"testing"

	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message"
)

type publisher struct {
	r *require.Assertions

	key     *ssb.KeyPair
	publish ssb.Publisher
}

func newPublisher(t *testing.T, root margaret.Log, users multilog.MultiLog) *publisher {
	r := require.New(t)
	kp, err := ssb.NewKeyPair(nil)
	r.NoError(err)
	return newPublisherWithKP(t, root, users, kp)
}

func newPublisherWithKP(t *testing.T, root margaret.Log, users multilog.MultiLog, kp *ssb.KeyPair) *publisher {
	p := &publisher{}
	p.r = require.New(t)
	p.key = kp

	var err error
	p.publish, err = message.OpenPublishLog(root, users, p.key)
	p.r.NoError(err)
	return p
}

func (p publisher) follow(ref *refs.FeedRef) {
	newSeq, err := p.publish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   ref.Ref(),
		"following": true,
	})
	p.r.NoError(err)
	p.r.NotNil(newSeq)
}

func (p publisher) unfollow(ref *refs.FeedRef) {
	newSeq, err := p.publish.Append(map[string]interface{}{
		"type":      "contact",
		"contact":   ref.Ref(),
		"following": false,
	})
	p.r.NoError(err)
	p.r.NotNil(newSeq)
}

func (p publisher) unblock(ref *refs.FeedRef) {
	newSeq, err := p.publish.Append(map[string]interface{}{
		"type":     "contact",
		"contact":  ref.Ref(),
		"blocking": false,
	})
	p.r.NoError(err)
	p.r.NotNil(newSeq)
}

func (p publisher) block(ref *refs.FeedRef) {
	newSeq, err := p.publish.Append(map[string]interface{}{
		"type":     "contact",
		"contact":  ref.Ref(),
		"blocking": true,
	})
	p.r.NoError(err)
	p.r.NotNil(newSeq)
}
