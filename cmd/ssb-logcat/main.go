// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
)

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	fmt.Fprintln(os.Stderr, "occurred at")
	debug.PrintStack()
	os.Exit(1)
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: ssb-logcat <repo> <startSeq> <limit>")
		os.Exit(1)
	}
	fromPath := os.Args[1]

	startSeqInt, err := strconv.Atoi(os.Args[2])
	check(err)
	startSeq := margaret.BaseSeq(startSeqInt)

	limit, err := strconv.Atoi(os.Args[3])
	check(err)

	repoFrom := repo.New(fromPath)

	from, err := repo.OpenLog(repoFrom)
	check(err)

	src, err := from.Query(margaret.Gt(startSeq), margaret.Limit(limit), margaret.SeqWrap(true))
	check(err)
	err = src.(luigi.PushSource).Push(context.TODO(), luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err == (luigi.EOS{}) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "push failed")
		}

		sw := v.(margaret.SeqWrapper)

		sv := sw.Value()

		msg, ok := sv.(refs.Message)
		if !ok {
			panic("wrong message type")
		}
		os.Stdout.WriteString(fmt.Sprintf(`
		{
			"key": %q,
			"rxSeq": %d,
			"value":
		`, msg.Key().Ref(), sw.Seq()))
		os.Stdout.Write(msg.ValueContentJSON())
		os.Stdout.WriteString("}\n")
		return err
	}))
	check(err)

}
