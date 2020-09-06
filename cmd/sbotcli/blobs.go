package main

import (
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/shurcooL/go-goon"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/blobstore"
	refs "go.mindeco.de/ssb-refs"
	"gopkg.in/urfave/cli.v2"
)

var blobsStore ssb.BlobStore

var blobsCmd = &cli.Command{
	Name: "blobs",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "localstore", Value: "", Usage: "non-remote repo allows for access withut involving a bot"},
	},
	Before: func(ctx *cli.Context) error {
		var localRepo = ctx.String("localstore")
		if localRepo == "" {
			//blobsStore, err = newClient(ctx)
			return errors.Errorf("TODO: implement more blobs features on client")
		}
		var err error
		blobsStore, err = blobstore.New(localRepo)
		if err != nil {
			return errors.Wrap(err, "blobs: failed to construct local edp")
		}
		log.Log("before", "blobs", "locl")
		return nil
	},
	Subcommands: []*cli.Command{
		blobsHasCmd,
		blobsWantCmd,
		blobsAddCmd,
		blobsGetCmd,
	},
}

var blobsHasCmd = &cli.Command{
	Name:  "has",
	Usage: "check if a blob is in the repo",
	Action: func(ctx *cli.Context) error {
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.has: need a blob ref")
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		// TODO: direct blobstore mode!?
		var has bool
		//sz, err := blobsStore.Size()
		resp, err := client.Async(longctx, has, muxrpc.Method{"blobs", "has"}, ref)
		if err != nil {
			return errors.Wrapf(err, "connect: async call failed.")
		}
		log.Log("event", "blob.has", "r", resp)
		goon.Dump(resp)
		var ok bool
		has, ok = resp.(bool)
		if !ok {
			return errors.Errorf("blobs.has: invalid return type: %T", resp)
		}

		if !has {
			log.Log("blob.has", false)
			os.Exit(1)
			return nil
		}
		return nil
	},
}

var blobsWantCmd = &cli.Command{
	Name:  "want",
	Usage: "try to get it from other peers",
	Action: func(ctx *cli.Context) error {
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.want: need a blob ref")
		}
		br, err := refs.ParseBlobRef(ref)
		if err != nil {
			return errors.Wrap(err, "blobs: failed to parse argument ref")
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}
		return client.BlobsWant(*br)
	},
}

var blobsAddCmd = &cli.Command{
	Name:  "add",
	Usage: "add a file to the store (use - to open stdin)",
	Action: func(ctx *cli.Context) error {
		if blobsStore == nil {
			return errors.Errorf("no blobstore use 'blobs --localstore $repo/blobs add -' for now")
		}
		fname := ctx.Args().Get(0)
		if fname == "" {
			return errors.New("blobs.add: need file to add (- for stdin)")
		}

		var rd io.Reader
		if fname == "-" {
			rd = os.Stdin
		} else {
			var err error
			rd, err = os.Open(fname)
			if err != nil {
				return errors.Wrap(err, "blobs.add: failed to open input file")
			}
		}

		ref, err := blobsStore.Put(rd)
		log.Log("blobs.add", ref.Ref())
		return err
	},
}

var blobsGetCmd = &cli.Command{
	Name:  "get",
	Usage: "prints the first argument to stdout",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "out", Value: "-", Usage: "where to? (stdout by default)"},
	},
	Action: func(ctx *cli.Context) error {
		if blobsStore == nil {
			return errors.Errorf("no blobstore use 'blobs --localstore $repo/blobs get &...' for now")
		}
		ref := ctx.Args().Get(0)
		if ref == "" {
			return errors.New("blobs.get: need a blob ref")
		}
		br, err := refs.ParseBlobRef(ref)
		if err != nil {
			return errors.Wrap(err, "blobs: failed to parse argument ref")
		}
		rd, err := blobsStore.Get(br)
		if err != nil {
			return errors.Wrap(err, "blobs: failed to parse argument ref")
		}

		var out io.Writer
		outName := ctx.String("out")
		if outName == "-" {
			out = os.Stdout
		} else {
			var err error
			out, err = os.Create(outName)
			if err != nil {
				return errors.Wrap(err, "blobs.get: failed to open output file")
			}
		}

		n, err := io.Copy(out, rd)
		log.Log("blobs.get", br.Ref(), "written", n)
		return err
	},
}
