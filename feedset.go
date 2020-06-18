// SPDX-License-Identifier: MIT

package ssb

import (
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	refs "go.mindeco.de/ssb-refs"
)

type strFeedMap map[librarian.Addr]struct{}

type StrFeedSet struct {
	mu  *sync.Mutex
	set strFeedMap
}

func NewFeedSet(size int) *StrFeedSet {
	return &StrFeedSet{
		mu:  new(sync.Mutex),
		set: make(strFeedMap, size),
	}
}

func (fs *StrFeedSet) AddStored(r *refs.StorageRef) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	b, err := r.Marshal()
	if err != nil {
		return errors.Wrap(err, "failed to marshal stored ref")
	}

	fs.set[librarian.Addr(b)] = struct{}{}
	return nil
}

func (fs *StrFeedSet) AddRef(ref *refs.FeedRef) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	copied := ref.Copy()

	fs.set[copied.StoredAddr()] = struct{}{}
	return nil
}

func (fs *StrFeedSet) Delete(ref *refs.FeedRef) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.set, ref.StoredAddr())
	return nil
}

func (fs *StrFeedSet) Count() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return len(fs.set)
}

func (fs StrFeedSet) List() ([]*refs.FeedRef, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var lst = make([]*refs.FeedRef, len(fs.set))

	i := 0

	for feed := range fs.set {
		var sr refs.StorageRef
		err := sr.Unmarshal([]byte(feed))
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode map entry")
		}
		got, err := sr.FeedRef()
		if err != nil {
			return nil, errors.Wrap(err, "failed to make ref from map entry")
		}
		// log.Printf("dbg List(%d) %s", i, ref.Ref())
		lst[i] = got.Copy()
		i++
	}
	return lst, nil
}

func (fs StrFeedSet) Has(ref *refs.FeedRef) bool {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	_, has := fs.set[ref.StoredAddr()]
	return has
}
