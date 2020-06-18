// SPDX-License-Identifier: MIT

package asynctesting

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
)

func CheckTypes(t *testing.T, tipe string, tExpected []string, rootLog margaret.Log, mt multilog.MultiLog) {
	r := require.New(t)

	typeLog, err := mt.Get(librarian.Addr(tipe))
	r.NoError(err, "error getting contacts log")
	typeLogQuery, err := typeLog.Query()
	r.NoError(err, "error querying contacts log")

	alice1Sink, wait := MakeCompareSink(tExpected, rootLog)
	err = luigi.Pump(context.TODO(), alice1Sink, typeLogQuery)
	r.NoError(err, "error pumping")
	err = alice1Sink.Close()
	r.NoError(err, "error closing sink")
	<-wait
}

func MakeCompareSink(texpected []string, rootLog margaret.Log) (luigi.FuncSink, chan struct{}) {
	var i int
	closeC := make(chan struct{})
	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		defer func() { i++ }()
		var expectEOS = i > len(texpected)-1
		if err != nil {
			if luigi.IsEOS(err) {
				if expectEOS {
					close(closeC)
					return nil
				}
				return errors.Errorf("expected more values after err %v  i%d, len%d", err, i, len(texpected))
			}
			return errors.Errorf("unexpected error %q", err)
		}

		v, err = rootLog.Get(v.(margaret.Seq))
		if err != nil {
			return errors.Errorf("error getting message from root log %q", v)
		}
		if expectEOS {
			return errors.Errorf("expected EOS but got value(%d) %v", i, v)
		}

		m := make(map[string]interface{})

		abs := v.(refs.Message)
		err = json.Unmarshal(abs.ContentBytes(), &m)
		if err != nil {
			return errors.Errorf("error decoding stored message %q", v)
		}
		if got := m["test"]; got != texpected[i] {
			return errors.Errorf("unexpected value %+v instead of %+v at i=%v", got, texpected[i], i)
		}

		return nil
	})

	return snk, closeC
}

func ServeLog(ctx context.Context, name string, l margaret.Log, f repo.ServeFunc) <-chan error {
	errc := make(chan error)
	go func() {
		err := f(ctx, l, true)
		if err != nil {
			errc <- errors.Wrapf(err, "%s serve exited", name)
		}
		close(errc)
	}()
	return errc
}

func MergedErrors(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error)

	output := func(c <-chan error) {
		for a := range c {
			out <- a
		}
		wg.Done()
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
