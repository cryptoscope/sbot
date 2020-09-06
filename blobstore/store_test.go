// SPDX-License-Identifier: MIT

package blobstore

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
)

func TestStore(t *testing.T) {
	type testcase struct {
		blobs   map[string]string
		putRefs []string
		delRefs []string
	}

	tcs := []testcase{
		{
			blobs: map[string]string{
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256": "omg",
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256": "wat",

				// these are in the same directory.
				// The second one is binary (git packfile),
				// so encoding it is a bit annoying.
				"&47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=.sha256": "",
				"&47hwmsDkBO4rXpJgiKY4dfJDoGB7oL/7wiimQsZL5wI=.sha256": "PACK" +
					"\x00\x00\x00\x02" +
					"\x00\x00\x00\x00" +
					"\x02\x9d\x08\x82" +
					"\x3b\xd8\xa8\xea" +
					"\xb5\x10\xad\x6a" +
					"\xc7\x5c\x82\x3c" +
					"\xfd\x3e\xd3\x1e",
			},
			putRefs: []string{
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256",
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256",
				"&47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=.sha256",
				"&47hwmsDkBO4rXpJgiKY4dfJDoGB7oL/7wiimQsZL5wI=.sha256",
			},
			delRefs: []string{
				"&ZR3jMW+ifnTWqd5hnrrGjjt4HpUn/dAMXvcUOx+lgbY=.sha256",
				"&8Ap4f3SSqV4WW0cHAvT+k3NYP73AJbLIvfAmLMSPz/Q=.sha256",
			},
		},
	}

	mkStore := func(name string) (ssb.BlobStore, func() error, error) {
		name = strings.Replace(name, "/", "_", -1)
		delBlobStore := func() error {
			return os.RemoveAll(name)
		}

		// remove debris of old test runs
		delBlobStore()

		store, err := New(name)
		return store, delBlobStore, err
	}

	mkTest := func(tc testcase) func(*testing.T) {
		var (
			iChangeSink int
		)

		changesSink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			defer func() { iChangeSink++ }()
			not, ok := v.(ssb.BlobStoreNotification)
			if !ok {
				return fmt.Errorf("expected %T but got %T", not, v)
			}

			if iChangeSink < len(tc.putRefs) {
				if not.Op != ssb.BlobStoreOpPut {
					return fmt.Errorf("expected op %q but got %q", ssb.BlobStoreOpPut, not.Op)
				}

				if tc.putRefs[iChangeSink] != not.Ref.Ref() {
					return fmt.Errorf("expected %#v but got %#v", tc.putRefs[iChangeSink], not.Ref.Ref())
				}
			} else if iChangeSink < len(tc.putRefs)+len(tc.delRefs) {
				if not.Op != ssb.BlobStoreOpRm {
					return fmt.Errorf("expected op %q but got %q", ssb.BlobStoreOpRm, not.Op)
				}

				i := iChangeSink - len(tc.putRefs)

				if tc.delRefs[i] != not.Ref.Ref() {
					return fmt.Errorf("expected %#v but got %#v", tc.putRefs[i], not.Ref.Ref())
				}

			} else {
				return fmt.Errorf("unexpected notification {Op:%v, Ref:%q}", not.Op, not.Ref.Ref())
			}

			return nil
		})

		return func(t *testing.T) {
			a := assert.New(t)
			r := require.New(t)

			bs, delBlobStore, err := mkStore(t.Name())
			r.NoError(err, "error making store")
			defer func() {
				if !t.Failed() {
					a.NoError(delBlobStore(), "error deleting blob store directory")
				}
			}()

			bs.Changes().Register(changesSink)

			testRefs := make(map[string]*refs.BlobRef)

			for _, refStr := range tc.putRefs {
				ref, err := bs.Put(strings.NewReader(tc.blobs[refStr]))
				r.NoError(err, "err putting blob %q", refStr)
				r.NotNil(ref, "ref returned by bs.Put is nil")
				a.Equal(refStr, ref.Ref(), "ref strings don't match: %q != %q", refStr, ref.Ref())
				testRefs[refStr] = ref

				sz, err := bs.Size(ref)
				r.NoError(err, "err getting blob size for %q", refStr)
				r.Equal(int64(len(tc.blobs[refStr])), sz, "blob size mismatch")

				rd, err := bs.Get(ref)
				r.NoError(err, "err getting blob %q", refStr)
				r.NotNil(rd, "reader returned by bs.Get is nil")

				data, err := ioutil.ReadAll(rd)
				r.NoError(err, "err reading blob %q", refStr)

				a.Equal(tc.blobs[refStr], string(data), "blob content mismatch")
			}

			ctx := context.Background()

			listExp := make(map[string]struct{})
			for _, refStr := range tc.putRefs {
				listExp[refStr] = struct{}{}
			}

			lstSrc := bs.List()
			for {
				v, err := lstSrc.Next(ctx)
				if luigi.IsEOS(err) {
					break
				} else {
					r.NoError(err, "error calling Next on list source")
				}

				ref, ok := v.(*refs.BlobRef)
				r.Equal(true, ok, "got something that is not a blobref in list: %v(%T)", v, v)

				_, ok = listExp[ref.Ref()]
				r.Equal(true, ok, "received unexpected ref in list:", ref)

				delete(listExp, ref.Ref())
			}

			r.Equal(0, len(listExp), "not all expected entries in list have been received: %v", listExp)

			for _, refStr := range tc.delRefs {
				ref := testRefs[refStr]
				err := bs.Delete(ref)
				r.NoError(err, "err putting blob %q", refStr)

				rd, err := bs.Get(ref)
				a.Equal(ErrNoSuchBlob, err, "unexpected error getting deleted blob %q", refStr)
				a.Equal(nil, rd, "got non-nil reader getting deleted blob %q", refStr)
			}
		}
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprint(i), mkTest(tc))
	}
}
