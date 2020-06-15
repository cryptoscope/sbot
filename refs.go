// SPDX-License-Identifier: MIT

package ssb

import (
	"net"

	"github.com/pkg/errors"

	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	refs "go.mindeco.de/ssb-refs"
)

type FeedRef struct {
	refs.FeedRef
}

type BlobRef struct {
	refs.BlobRef
}

type MessageRef struct {
	refs.MessageRef
}

// GetFeedRefFromAddr uses netwrap to get the secretstream address and then uses ParseFeedRef
func GetFeedRefFromAddr(addr net.Addr) (*FeedRef, error) {
	addr = netwrap.GetAddr(addr, secretstream.NetworkString)
	if addr == nil {
		return nil, errors.New("no shs-bs address found")
	}
	ssAddr := addr.(secretstream.Addr)
	return refs.ParseFeedRef(ssAddr.String())
}
