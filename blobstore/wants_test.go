// SPDX-License-Identifier: MIT

package blobstore

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	mmock "go.cryptoscope.co/muxrpc/mock"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func TestWantManager(t *testing.T) {
	type testcase struct {
		blobs map[string]string // global key-value register of all blobs

		localBlobs     []string
		localLateBlobs []string

		localWants  map[string]int64
		remoteWants map[string]int64
	}

	tcs := []testcase{
		{
			blobs: map[string]string{
				"&6EcSI4cJOY9tNJ3CJQsO/KS3LYwr+3t0M50wupQFaxQ=.sha256": "ohai",
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": "wat",
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256": "omg",
				"&47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=.sha256": "",
			},

			localBlobs:     []string{""},
			localLateBlobs: []string{"wat"}, // &8Ap4...
			localWants: map[string]int64{
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256": -1, // "omg"
				"&6EcSI4cJOY9tNJ3CJQsO/KS3LYwr+3t0M50wupQFaxQ=.sha256": -1, // "ohai"
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": -2, // replicating the wat want
			},
			remoteWants: map[string]int64{
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": -1, // "wat"
				"&47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=.sha256": -1, // ""
				"&6EcSI4cJOY9tNJ3CJQsO/KS3LYwr+3t0M50wupQFaxQ=.sha256": 4,  // "ohai"
			},
		},
	}

	mkStore := func(name string) (ssb.BlobStore, func() error, error) {
		var err error
		name = strings.Replace(name, "/", "_", -1)
		name, err = ioutil.TempDir("", name)
		if err != nil {
			return nil, nil, err
		}
		delBlobStore := func() error {
			return os.RemoveAll(name)
		}

		// remove debris of old test runs
		delBlobStore()

		store, err := New(name)
		return store, delBlobStore, err
	}

	mkTest := func(tc testcase) func(*testing.T) {
		return func(t *testing.T) {
			t.Log("Warning: currectly we only log errors that happen " +
				"when the node tries to fetch a blob the peer currently " +
				"has. TODO: Find a good way to make the test fail instead.")
			a := assert.New(t)
			r := require.New(t)

			bs, delBlobStore, err := mkStore(t.Name())
			r.NoError(err, "error creating want manager")
			defer func() {
				if !t.Failed() {
					a.NoError(delBlobStore(), "error deleting blob store directory")
				}
			}()

			log := testutils.NewRelativeTimeLogger(nil)
			wmgr := NewWantManager(log, bs)

			for _, str := range tc.localBlobs {
				br, err := bs.Put(strings.NewReader(str))
				r.NoError(err)
				r.NotNil(br)
			}

			for refStr, dist := range tc.localWants {
				ref, err := parseBlobRef(refStr)
				r.NoError(err, "error parsing ref %q", ref)

				a.NoError(wmgr.WantWithDist(ref, dist), "error wanting local ref")
			}

			for refStr := range tc.localWants {
				ref, err := parseBlobRef(refStr)
				r.NoError(err, "error parsing ref %q", ref)

				a.Equal(true, wmgr.Wants(ref), "expected want manager to want ref %q, but it doesn't", ref.Ref())
			}

			var wmsg WantMsg
			for refStr, dist := range tc.remoteWants {
				ref, err := parseBlobRef(refStr)
				r.NoError(err, "error parsing ref %q", ref)

				wmsg = append(wmsg, ssb.BlobWant{Ref: ref, Dist: dist})
			}

			var outSlice []interface{}
			out := luigi.NewSliceSink(&outSlice)
			ctx := context.Background()
			edp := &mmock.FakeEndpoint{
				SourceStub: func(ctx context.Context, tipe interface{}, method muxrpc.Method, args ...interface{}) (luigi.Source, error) {
					if len(args) != 1 {
						return nil, errors.Errorf("expected one argument, got %v", len(args))
					}

					refStr, ok := args[0].(string)
					if !ok {
						return nil, errors.Errorf("expected a string argument, got type %T", args[0])
					}

					br, err := parseBlobRef(refStr)
					if err != nil {
						return nil, errors.Wrap(err, "error parsing blob ref")
					}

					sz, ok := tc.remoteWants[br.Ref()]
					if !ok || sz < 0 {
						return nil, ErrNoSuchBlob
					}

					data := tc.blobs[br.Ref()]

					return (*luigi.SliceSource)(&[]interface{}{
						[]byte(data),
					}), nil
				},
				RemoteStub: func() net.Addr {
					return &net.TCPAddr{Port: 666}
				},
			}
			proc := wmgr.CreateWants(ctx, out, edp)
			err = proc.Pour(ctx, &wmsg)
			r.NoError(err, "error pouring first want message")

			sizeWants := func(strs []string) map[string]int64 {
				var (
					m   = make(map[string]int64)
					h   = sha256.New()
					ref = refs.BlobRef{
						Algo: ssb.RefAlgoBlobSSB1,
					}
				)

				for _, str := range strs {
					h.Reset()
					h.Write([]byte(str))
					ref.Hash = h.Sum(nil)
					m[ref.Ref()] = int64(len(str))
				}

				return m
			}

			// TODO this is pretty specific to the only test case defined above.
			// would be nice to generalize this further so we can add more cases.

			// should contain our wants and our size response to their want
			r.Equal(2, len(outSlice), "output slice length mismatch (%v)", outSlice)

			// this should be our initial want list, but with more dist
			ourW := wmgr.(*wantManager)
			ourW.l.Lock()
			a.IsType(map[string]int64{}, outSlice[0], "slice element type mismatch")
			a.Equal(tc.localWants, outSlice[0], "map content mismatch")

			a.IsType(map[string]int64{}, outSlice[1], "slice element type mismatch")
			a.Equal(sizeWants(tc.localBlobs), outSlice[1], "map content mismatch")
			ourW.l.Unlock()

			for _, str := range tc.localLateBlobs {
				_, err := bs.Put(strings.NewReader(str))
				a.NoError(err, "error putting blob")
			}

			ourW.l.Lock()
			// should contain our wants and our size response to their want
			r.Equal(3, len(outSlice), "output slice length mismatch")

			a.IsType(map[string]int64{}, outSlice[2], "slice element type mismatch")
			a.Equal(sizeWants(tc.localLateBlobs), outSlice[2], "map content mismatch")
			ourW.l.Unlock()
		}
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprint(i), mkTest(tc))
	}
}
