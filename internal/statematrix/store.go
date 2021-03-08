// SPDX-License-Identifier: MIT

/*Package statematrix stores and provides useful operations on an state matrix for a epidemic broadcast tree protocol.

The state matrix represents multiple _network frontiers_ (or vector clock).

This version uses a SQL because that seems much handier to handle such an irregular sparse matrix.

Q:
* do we need a 2nd _told us about_ table?

*/
package statematrix

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
	"go.mindeco.de/ssb-refs/tfk"
)

type StateMatrix struct {
	basePath string

	self string // whoami

	mu   sync.Mutex
	open currentFrontiers
}

// map[peer refernce]frontier
type currentFrontiers map[string]ssb.NetworkFrontier

func New(base string, self *refs.FeedRef) (*StateMatrix, error) {

	os.MkdirAll(base, 0700)

	sm := StateMatrix{
		basePath: base,

		self: self.Ref(),

		open: make(currentFrontiers),
	}

	_, err := sm.loadFrontier(self)
	if err != nil {
		return nil, err
	}

	return &sm, nil
}

// Inspect returns the current frontier for the passed peer
func (sm *StateMatrix) Inspect(peer *refs.FeedRef) (ssb.NetworkFrontier, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.loadFrontier(peer)
}

func (sm *StateMatrix) StateFileName(peer *refs.FeedRef) (string, error) {
	peerTfk, err := tfk.Encode(peer)
	if err != nil {
		return "", err
	}

	peerFileName := filepath.Join(sm.basePath, fmt.Sprintf("%x", peerTfk))
	return peerFileName, nil
}

func (sm *StateMatrix) loadFrontier(peer *refs.FeedRef) (ssb.NetworkFrontier, error) {
	curr, has := sm.open[peer.Ref()]
	if has {
		return curr, nil
	}

	peerFileName, err := sm.StateFileName(peer)
	if err != nil {
		return nil, err
	}

	peerFile, err := os.Open(peerFileName)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		curr = make(ssb.NetworkFrontier)
	} else {
		defer peerFile.Close()

		var nf ssb.NetworkFrontier
		err = json.NewDecoder(peerFile).Decode(&nf)
		if err != nil {
			return nil, err
		}

		curr = nf
	}

	sm.open[peer.Ref()] = curr
	return curr, nil
}

func (sm *StateMatrix) SaveAndClose(peer *refs.FeedRef) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.saveAndClose(peer.Ref())
}

func (sm *StateMatrix) saveAndClose(peer string) error {
	parsed, err := refs.ParseFeedRef(peer)
	if err != nil {
		return err
	}

	err = sm.save(parsed)
	if err != nil {
		return err
	}

	delete(sm.open, peer)
	return nil
}

func (sm *StateMatrix) save(peer *refs.FeedRef) error {
	peerFileName, err := sm.StateFileName(peer)
	if err != nil {
		return err
	}

	// truncate the file for overwriting, create it if it doesnt exist
	peerFile, err := os.OpenFile(peerFileName, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0700)
	if err != nil {
		return err
	}

	nf, has := sm.open[peer.Ref()]
	if !has {
		return nil
	}

	err = json.NewEncoder(peerFile).Encode(nf)
	if err != nil {
		return err
	}

	err = peerFile.Close()
	if err != nil {
		return err
	}

	return nil
}

type HasLongerResult struct {
	Peer *refs.FeedRef
	Feed *refs.FeedRef
	Len  uint64
}

func (hlr HasLongerResult) String() string {
	return fmt.Sprintf("%s: %s:%d", hlr.Peer.ShortRef(), hlr.Feed.ShortRef(), hlr.Len)
}

// HasLonger returns all the feeds which have more messages then we have and who has them.
func (sm *StateMatrix) HasLonger() ([]HasLongerResult, error) {
	var err error

	sm.mu.Lock()
	defer sm.mu.Unlock()

	selfNf, has := sm.open[sm.self]
	if !has {
		return nil, nil
	}

	var res []HasLongerResult

	for peer, theirNf := range sm.open {

		for feed, note := range selfNf {

			theirNote, has := theirNf[feed]
			if !has {
				continue
			}

			if theirNote.Seq > note.Seq {
				var hlr HasLongerResult
				hlr.Len = uint64(theirNote.Seq)

				hlr.Peer, err = refs.ParseFeedRef(peer)
				if err != nil {
					return nil, err
				}

				hlr.Feed, err = refs.ParseFeedRef(feed)
				if err != nil {
					return nil, err
				}

				res = append(res, hlr)
			}

		}
	}

	return res, nil
}

// WantsList returns all the feeds a peer wants to recevie messages for
func (sm *StateMatrix) WantsList(peer *refs.FeedRef) ([]*refs.FeedRef, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nf, err := sm.loadFrontier(peer)
	if err != nil {
		return nil, err
	}

	var res []*refs.FeedRef

	for feedStr, note := range nf {
		if note.Receive {
			feed, err := refs.ParseFeedRef(feedStr)
			if err != nil {
				return nil, fmt.Errorf("wantList: failed to parse feed entry %q: %w", feedStr, err)
			}
			res = append(res, feed)
		}
	}

	return res, nil
}

// WantsFeed returns true if peer want's to receive feed
func (sm *StateMatrix) WantsFeed(peer, feed *refs.FeedRef) (bool, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nf, err := sm.loadFrontier(peer)
	if err != nil {
		return false, err
	}

	n, has := nf[feed.Ref()]
	if !has {
		return false, nil
	}

	return n.Receive, nil
}

// Changed returns which feeds have newer messages since last update
func (sm *StateMatrix) Changed(self, peer *refs.FeedRef) (ssb.NetworkFrontier, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var err error

	selfNf, err := sm.loadFrontier(self)
	if err != nil {
		return nil, err
	}

	// no state yet
	if len(selfNf) == 0 {
		// use the replication lister and determin the stored feeds lenghts
		return selfNf, nil
		/* TODO: move zero-state code somewhere else
		lister := sm.wantList.ReplicationList()
		feeds, err := lister.List()
		if err != nil {
			return nil, fmt.Errorf("failed to get userlist: %w", err)
		}

		for i, feed := range feeds {
			if feed.Algo != refs.RefAlgoFeedSSB1 {
				// skip other formats (TODO: gg support)
				continue
			}

			seq, err := sm.currSeq.CurrentSequence(feed)
			if err != nil {
				return nil, fmt.Errorf("failed to get sequence for entry %d: %w", i, err)
			}
			selfNf[feed.Ref()] = seq
		}

		selfNf[self.Ref()], err = sm.currSeq.CurrentSequence(self)
		if err != nil {
			return nil, fmt.Errorf("failed to get our sequence: %w", err)
		}

		sm.open[sm.self] = selfNf
		err = sm.save(self)
		if err != nil {
			return nil, err
		}
		*/
	}

	peerNf, err := sm.loadFrontier(peer)
	if err != nil {
		fmt.Println("ebt/warning: remote peer state loading error:", err)
		return selfNf, nil
	}

	// just the ones that differ
	changedFrontier := make(ssb.NetworkFrontier)

	for myFeed, myNote := range selfNf {
		theirNote, has := peerNf[myFeed]
		if !has && myNote.Receive {
			// they don't have it, but tell them we want it
			changedFrontier[myFeed] = myNote
			continue
		}

		if !theirNote.Receive {
			// they dont care about this feed
			continue
		}

		if myNote.Seq > theirNote.Seq {
			// we have more then them, tell them how much
			changedFrontier[myFeed] = myNote
		}
	}

	return changedFrontier, nil
}

type ObservedFeed struct {
	Feed *refs.FeedRef

	ssb.Note
}

// Update gets the current state from who, overwrites the notes in current with the new ones from the passed update
// and returns the complet updated frontier.
func (sm *StateMatrix) Update(who *refs.FeedRef, update ssb.NetworkFrontier) (ssb.NetworkFrontier, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	current, err := sm.loadFrontier(who)
	if err != nil {
		return nil, err
	}

	// overwrite the entries in current with the updated ones
	for feed, note := range update {
		current[feed] = note
	}

	sm.open[who.Ref()] = current
	return current, nil
}

// Fill might be deprecated. It just updates the current frontier state
func (sm *StateMatrix) Fill(who *refs.FeedRef, feeds []ObservedFeed) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	nf, err := sm.loadFrontier(who)
	if err != nil {
		return err
	}

	for _, of := range feeds {

		if of.Replicate {
			nf[of.Feed.Ref()] = of.Note
		} else {
			// seq == -1 means drop it
			delete(nf, of.Feed.Ref())
		}
	}

	sm.open[who.Ref()] = nf
	return nil
}

func (sm *StateMatrix) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for peer := range sm.open {
		sm.saveAndClose(peer)
	}

	return nil
}
