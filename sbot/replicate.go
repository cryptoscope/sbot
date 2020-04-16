package sbot

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/graph"
)

var _ ssb.Replicator = (*Sbot)(nil)

type replicator struct {
	builder graph.Builder
	current *lister
}

func (s *Sbot) newGraphReplicator() (*replicator, error) {
	var r replicator
	r.builder = s.GraphBuilder
	r.current = newLister()

	replicateEvt := log.With(s.info, "event", "update-replicate")
	update := r.makeUpdater(replicateEvt, s.KeyPair.Id, int(s.hopCount))

	// update for new messages but only every 15seconds
	go debounce(s.rootCtx, 15*time.Second, s.RootLog.Seq(), update)

	return &r, nil
}

// makeUpdater returns a func that does the hop-walk and block checks, used together with debounce
func (r *replicator) makeUpdater(log log.Logger, self *ssb.FeedRef, hopCount int) func() {
	return func() {
		start := time.Now()
		newWants := r.builder.Hops(self, hopCount)
		level.Debug(log).Log("feed-want-count", newWants.Count(), "hops", hopCount, "took", time.Since(start))

		refs, err := newWants.List()
		if err != nil {
			level.Error(log).Log("msg", "want list failed", "err", err, "wants", newWants.Count())
			return
		}
		for _, ref := range refs {
			r.current.feedWants.AddRef(ref)
		}

		// make sure we dont fetch and allow blocked feeds
		g, err := r.builder.Build()
		if err != nil {
			level.Error(log).Log("msg", "failed to build blocks", "err", err)
			return
		}

		r.current.blocked = g.BlockedList(self)
		lst, err := r.current.blocked.List()
		if err == nil {
			for _, bf := range lst {
				r.current.feedWants.Delete(bf)
			}
		}
	}
}

func debounce(ctx context.Context, interval time.Duration, obs luigi.Observable, work func()) {
	var seq = margaret.SeqEmpty
	timer := time.NewTimer(interval)

	handle := luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if err != nil {
			return err
		}
		newSeq, ok := val.(margaret.BaseSeq)
		if !ok {
			return fmt.Errorf("graph rebuild debounce: wrong type: %T", val)
		}
		seq = newSeq
		timer.Reset(interval)
		return nil
	})
	done := obs.Register(handle)

	for {
		select {
		case <-ctx.Done():
			done()
			return

		case <-timer.C:
			if seq != margaret.SeqEmpty {
				work()
				seq = margaret.SeqEmpty
			}
		}
	}
}

func (r *replicator) Block(ref *ssb.FeedRef)   { r.current.blocked.AddRef(ref) }
func (r *replicator) Unblock(ref *ssb.FeedRef) { r.current.blocked.Delete(ref) }

func (r *replicator) Replicate(ref *ssb.FeedRef)     { r.current.feedWants.AddRef(ref) }
func (r *replicator) DontReplicate(ref *ssb.FeedRef) { r.current.feedWants.Delete(ref) }

func (r *replicator) makeLister() ssb.ReplicationLister { return r.current }

type lister struct {
	feedWants *ssb.StrFeedSet
	blocked   *ssb.StrFeedSet
}

func newLister() *lister {
	return &lister{
		feedWants: ssb.NewFeedSet(0),
		blocked:   ssb.NewFeedSet(0),
	}
}

func (l lister) Authorize(remote *ssb.FeedRef) error {
	if l.blocked.Has(remote) {
		return fmt.Errorf("peer blocked")
	}

	if l.feedWants.Has(remote) {
		return nil
	}
	return errors.New("nope - access denied")
}

func (l lister) ReplicationList() *ssb.StrFeedSet { return l.feedWants }
func (l lister) BlockList() *ssb.StrFeedSet       { return l.blocked }
