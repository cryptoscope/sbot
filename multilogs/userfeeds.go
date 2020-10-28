// SPDX-License-Identifier: MIT

package multilogs

import (
	"context"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

const IndexNameFeeds = "userFeeds"

func OpenUserFeeds(r repo.Interface) (*roaring.MultiLog, librarian.SinkIndex, error) {
	return repo.OpenFileSystemMultiLog(r, IndexNameFeeds, UserFeedsUpdate)
}

func UserFeedsUpdate(ctx context.Context, seq margaret.Seq, value interface{}, mlog multilog.MultiLog) error {
	if nulled, ok := value.(error); ok {
		if margaret.IsErrNulled(nulled) {
			return nil
		}
		return nulled
	}

	abstractMsg, ok := value.(refs.Message)
	if !ok {
		return errors.Errorf("error casting message. got type %T", value)
	}

	author := abstractMsg.Author()
	if author == nil {
		return errors.Errorf("nil author on message?! %v (%d)", value, seq.Seq())
	}

	authorLog, err := mlog.Get(author.StoredAddr())
	if err != nil {
		return errors.Wrap(err, "error opening sublog")
	}

	_, err = authorLog.Append(seq)
	return errors.Wrap(err, "error appending new author message")
}
