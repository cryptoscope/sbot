// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	cli "gopkg.in/urfave/cli.v2"
)

var streamFlags = []cli.Flag{
	&cli.IntFlag{Name: "limit", Value: -1},
	&cli.IntFlag{Name: "seq", Value: 0},
	&cli.BoolFlag{Name: "reverse"},
	&cli.BoolFlag{Name: "live"},
	&cli.BoolFlag{Name: "keys", Value: false},
	&cli.BoolFlag{Name: "values", Value: false},
}

type mapMsg map[string]interface{}

var typeStreamCmd = &cli.Command{
	Name:  "partial",
	Flags: append(streamFlags, &cli.StringFlag{Name: "id"}, &cli.BoolFlag{Name: "asJSON"}),
	Action: func(ctx *cli.Context) error {

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		ir, err := client.Whoami()
		if err != nil {
			return err
		}
		id := ir.Ref()
		if f := ctx.String("id"); f != "" {
			id = f
		}

		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"partial"}, struct {
			ID   string `json:"id"`
			Tipe string `json:"type"`
		}{
			ID:   id,
			Tipe: ctx.Args().First(),
		})
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "byType failed")
	},
}

var historyStreamCmd = &cli.Command{
	Name:  "hist",
	Flags: append(streamFlags, &cli.StringFlag{Name: "id"}, &cli.BoolFlag{Name: "asJSON"}),
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		ir, err := client.Whoami()
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		args.ID = ir
		if f := ctx.String("id"); f != "" {
			flagRef, err := ssb.ParseFeedRef(f)
			if err != nil {
				return err
			}
			args.ID = flagRef
		}
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"createHistoryStream"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "feed hist failed")
	},
}

var logStreamCmd = &cli.Command{
	Name:  "log",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"createLogStream"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "log failed")
	},
}

var privateReadCmd = &cli.Command{
	Name:  "read",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"private", "read"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "private/read failed")
	},
}

var replicateUptoCmd = &cli.Command{
	Name:  "upto",
	Flags: streamFlags,
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var args = getStreamArgs(ctx)
		src, err := client.Source(longctx, mapMsg{}, muxrpc.Method{"replicate", "upto"}, args)
		if err != nil {
			return errors.Wrap(err, "source stream call failed")
		}
		err = luigi.Pump(longctx, jsonDrain(os.Stdout), src)
		return errors.Wrap(err, "replicate/upto failed")
	},
}

func jsonDrain(w io.Writer) luigi.Sink {
	i := 0
	return luigi.FuncSink(func(ctx context.Context, val interface{}, err error) error {
		if luigi.IsEOS(err) {
			return nil
		} else if err != nil {
			return errors.Wrapf(err, "jsonDrain: failed to drain message %d", i)
		}
		b, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "jsonDrain: failed to encode msg %d", i)
		}
		_, err = fmt.Fprintln(w, string(b))
		if err != nil {
			return errors.Wrapf(err, "jsonDrain: failed to write msg %d", i)
		}
		i++
		return nil
	})
}

/*

func query(ctx *cli.Context) error {
	reply := make(chan map[string]interface{})
	go func() {
		for r := range reply {
			goon.Dump(r)
		}
	}()
	if err := client.Source("query.read", reply, ctx.Args().Get(0)); err != nil {
		return errors.Wrap(err, "source stream call failed")
	}
	return client.Close()
}
*/
