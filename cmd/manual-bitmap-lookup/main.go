// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/RoaringBitmap/roaring"
	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/margaret/multilog/roaring/fs"
	"go.cryptoscope.co/margaret/offset2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/multimsg"
)

var check = logging.CheckFatal

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <dir> (hasAddr)", os.Args[0])
		os.Exit(1)
	}
	logging.SetupLogging(nil)
	log := logging.Logger(os.Args[0])

	dir := os.Args[1]

	mlog, err := fs.NewMultiLog(dir)
	check(errors.Wrap(err, "error opening database"))

	addrs, err := mlog.List()
	check(errors.Wrap(err, "error listing multilog"))
	log.Log("mlog", "opened", "list#", len(addrs))

	var sorted = make(countedAddrs, len(addrs))
	for i, a := range addrs {
		bmap, err := mlog.LoadInternalBitmap(a)
		check(err)

		sorted[i] = countedAddr{
			addr: a,
			c:    bmap.GetCardinality(),
			bmap: bmap,
		}

	}
	sort.Sort(countedAddrs(sorted))

	for i, a := range sorted {
		fmt.Println(i, a.addr, a.c)
	}

	// check has
	if len(os.Args) > 2 {
		addr := librarian.Addr(os.Args[2])
		has, err := multilog.Has(mlog, addr)
		log.Log("mlog", "has", "addr", addr, "has?", has, "hasErr", err)

		if has {
			bmap, err := mlog.LoadInternalBitmap(addr)
			check(err)

			fmt.Println("count:", bmap.GetCardinality())
			fmt.Println(bmap.String())
			logp := "/home/cryptix/.ssb-go/log"
			log, err := offset2.Open(logp, multimsg.MargaretCodec{})
			check(err)

			it := bmap.Iterator()
			for it.HasNext() {
				seq := margaret.BaseSeq(it.Next())
				sv, err := log.Get(seq)
				if err != nil {
					check(err)
				}
				msg := sv.(ssb.Message)
				fmt.Println(string(msg.ValueContentJSON()))
			}
			check(log.Close())
		}
	}

	check(mlog.Close())
}

type countedAddr struct {
	addr librarian.Addr
	c    uint64
	bmap *roaring.Bitmap
}

type countedAddrs []countedAddr

// Len is the number of elements in the collection.
func (a countedAddrs) Len() int {
	return len(a)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (a countedAddrs) Less(i int, j int) bool {
	return a[i].c < a[j].c
}

// Swap swaps the elements with indexes i and j.
func (a countedAddrs) Swap(i int, j int) {
	a[i], a[j] = a[j], a[i]
}
