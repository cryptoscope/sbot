package invite

import (
	"bytes"
	"net"
	"testing"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestParseParseLegacyToken(t *testing.T) {
	a := assert.New(t)

	testRef := refs.FeedRef{
		ID:   bytes.Repeat([]byte("b00p"), 8),
		Algo: ssb.RefAlgoFeedSSB1,
	}
	var tcases = []struct {
		input string
		err   error
		want  *Token
	}{
		{"127.0.0.1:123", ErrInvalidToken, nil},
		{"127.0.0.1:39979:@w50PNhe69skpWDWwitpik/hq3hrFl6PxZY/tUK+XDpM=.ed25519", ErrInvalidToken, nil},
		{"@xxx.foo", ErrInvalidToken, nil},

		{"255.1.1.255:666:@YjAwcGIwMHBiMDBwYjAwcGIwMHBiMDBwYjAwcGIwMHA=.ed25519~AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", nil, &Token{
			Address: netwrap.WrapAddr(&net.TCPAddr{
				IP:   net.ParseIP("255.1.1.255"),
				Port: 666,
			}, secretstream.Addr{testRef.ID}),
			Peer: testRef,
			Seed: [32]byte{},
		}},

		// v6
		{"[fc97:c693:8b07:f84e:cbbf:d89a:16d5:3630]:1234:@YjAwcGIwMHBiMDBwYjAwcGIwMHBiMDBwYjAwcGIwMHA=.ed25519~AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", nil, &Token{
			Address: netwrap.WrapAddr(&net.TCPAddr{
				IP:   net.ParseIP("fc97:c693:8b07:f84e:cbbf:d89a:16d5:3630"),
				Port: 1234,
			}, secretstream.Addr{testRef.ID}),
			Peer: testRef,
			Seed: [32]byte{},
		}},

		// hostnames
		{"localhost:666:@YjAwcGIwMHBiMDBwYjAwcGIwMHBiMDBwYjAwcGIwMHA=.ed25519~AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", nil, &Token{
			Address: netwrap.WrapAddr(&net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 666,
			}, secretstream.Addr{testRef.ID}),
			Peer: testRef,
			Seed: [32]byte{},
		}},
	}
	for i, tc := range tcases {
		tok, err := ParseLegacyToken(tc.input)
		if tc.err == nil {
			if !a.NoError(err, "got error on test %d (%v)", i, tc.input) {
				continue
			}

			a.Equal(tok.String(), tc.want.String(), "test %d input<>output failed", i)
		} else {
			a.EqualError(errors.Cause(err), tc.err.Error(), "%d wrong error", i)
		}
	}
}
