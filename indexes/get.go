// SPDX-License-Identifier: MIT

package indexes

import (
	"context"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

const FolderNameGet = "get"

// OpenGet supplies the get(msgRef) -> rootLogSeq idx
func OpenGet(r repo.Interface) (librarian.Index, librarian.SinkIndex, error) {
	_, idx, sinkIdx, err := repo.OpenBadgerIndex(r, FolderNameGet, updateFn)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting get() index")
	}
	return idx, sinkIdx, nil
}

func updateFn(db *badger.DB) (librarian.SeqSetterIndex, librarian.SinkIndex) {
	idx := libbadger.NewIndex(db, margaret.BaseSeq(0))
	sink := librarian.NewSinkIndex(func(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
		msg, ok := val.(refs.Message)
		if !ok {
			err, ok := val.(error)
			if ok && margaret.IsErrNulled(err) {
				return nil
			}
			return errors.Errorf("index/get: unexpected message type: %T", val)
		}
		err := idx.Set(ctx, librarian.Addr(msg.Key().Hash), seq.Seq())
		return errors.Wrapf(err, "index/get: failed to update message %s (seq: %d)", msg.Key().Ref(), seq.Seq())
	}, idx)
	return idx, sink
}
