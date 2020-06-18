// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"

	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret/multilog"
	multimkv "go.cryptoscope.co/margaret/multilog/roaring/mkv"
	refs "go.mindeco.de/ssb-refs"
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

	mlog, err := multimkv.NewMultiLog(dir)
	check(err)

	/*
		addrs, err := mlog.List()
		check(errors.Wrap(err, "error listing multilog"))
		log.Log("mlog", "opened", "list#", len(addrs))
		for i, a := range addrs {
			var sr ssb.StorageRef
			err := sr.Unmarshal([]byte(a))
			check(err)

			sublog, err := mlog.Get(a)
			check(err)
			seqv, err := sublog.Seq().Value()
			check(err)
			log.Log("i", i, "addr", sr.Ref(), "seq", seqv)
		}
	*/

	// check has
	if len(os.Args) > 2 {
		ref, err := refs.ParseFeedRef(os.Args[2])
		check(err)
		has, err := multilog.Has(mlog, ref.StoredAddr())
		log.Log("mlog", "has", "addr", ref.Ref(), "has?", has, "hasErr", err)

		bmap, err := mlog.LoadInternalBitmap(ref.StoredAddr())
		check(err)
		fmt.Println(bmap.GetCardinality())
		fmt.Println(bmap.String())
	}

	check(mlog.Close())
}
