// SPDX-License-Identifier: MIT

// sbotcli implements a simple tool to query commands on another sbot
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/log/term"
	"github.com/pkg/errors"
	goon "github.com/shurcooL/go-goon"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
	"go.cryptoscope.co/ssb"
	ssbClient "go.cryptoscope.co/ssb/client"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"
	cli "gopkg.in/urfave/cli.v2"
)

// Version and Build are set by ldflags
var (
	Version = "snapshot"
	Build   = ""
)

var (
	longctx      context.Context
	shutdownFunc func()

	log kitlog.Logger

	keyFileFlag  = cli.StringFlag{Name: "key,k", Value: "unset"}
	unixSockFlag = cli.StringFlag{Name: "unixsock", Usage: "if set, unix socket is used instead of tcp"}
)

func init() {
	u, err := user.Current()
	check(err)

	keyFileFlag.Value = filepath.Join(u.HomeDir, ".ssb-go", "secret")
	unixSockFlag.Value = filepath.Join(u.HomeDir, ".ssb-go", "socket")

	log = term.NewColorLogger(os.Stdout, kitlog.NewLogfmtLogger, colorFn)
}

var app = cli.App{
	Name:    os.Args[0],
	Usage:   "client for controlling Cryptoscope's SSB server",
	Version: "alpha4",

	Flags: []cli.Flag{
		&cli.StringFlag{Name: "shscap", Value: "1KHLiKZvAvjbY1ziZEHMXawbCEIM6qwjCDm3VYRan/s=", Usage: "shs key"},
		&cli.StringFlag{Name: "addr", Value: "localhost:8008", Usage: "tcp address of the sbot to connect to (or listen on)"},
		&cli.StringFlag{Name: "remoteKey", Value: "", Usage: "the remote pubkey you are connecting to (by default the local key)"},
		&keyFileFlag,
		&unixSockFlag,
		&cli.BoolFlag{Name: "verbose,vv", Usage: "print muxrpc packets"},
	},

	Before: initClient,
	Commands: []*cli.Command{
		blobsCmd,
		blockCmd,
		friendsCmd,
		logStreamCmd,
		sortedStreamCmd,
		typeStreamCmd,
		historyStreamCmd,
		partialStreamCmd,
		replicateUptoCmd,
		repliesStreamCmd,
		callCmd,
		getCmd,
		connectCmd,
		publishCmd,
		groupsCmd,
	},
}

// Color by level
func colorFn(keyvals ...interface{}) term.FgBgColor {
	for i := 1; i < len(keyvals); i += 2 {
		if _, ok := keyvals[i].(error); ok {
			return term.FgBgColor{Fg: term.Red}
		}
	}
	return term.FgBgColor{}
}

func check(err error) {
	if err != nil {
		level.Error(log).Log("err", err)
		os.Exit(1)
	}
}

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s (rev: %s, built: %s)\n", c.App.Version, Version, Build)
	}

	if err := app.Run(os.Args); err != nil {
		level.Error(log).Log("run-failure", err)
	}
}

func todo(ctx *cli.Context) error {
	return errors.Errorf("todo: %s", ctx.Command.Name)
}

func initClient(ctx *cli.Context) error {
	longctx = context.Background()
	longctx, shutdownFunc = context.WithTimeout(longctx, 45*time.Second)
	signalc := make(chan os.Signal)
	signal.Notify(signalc, os.Interrupt, syscall.SIGTERM)
	go func() {
		s := <-signalc
		level.Warn(log).Log("event", "shutting down", "sig", s)
		shutdownFunc()
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	return nil
}

func newClient(ctx *cli.Context) (*ssbClient.Client, error) {
	sockPath := ctx.String("unixsock")
	if sockPath != "" {
		client, err := ssbClient.NewUnix(sockPath, ssbClient.WithContext(longctx))
		if err != nil {
			return nil, errors.Wrap(err, "unix-path based client init failed")
		}
		return client, nil
	}

	// Assume TCP connection
	localKey, err := ssb.LoadKeyPair(ctx.String("key"))
	if err != nil {
		return nil, err
	}

	var remotePubKey = make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(remotePubKey, localKey.Pair.Public)
	if rk := ctx.String("remoteKey"); rk != "" {
		rk = strings.TrimSuffix(rk, ".ed25519")
		rk = strings.TrimPrefix(rk, "@")
		rpk, err := base64.StdEncoding.DecodeString(rk)
		if err != nil {
			return nil, errors.Wrapf(err, "init: base64 decode of --remoteKey failed")
		}
		copy(remotePubKey, rpk)
	}

	plainAddr, err := net.ResolveTCPAddr("tcp", ctx.String("addr"))
	if err != nil {
		return nil, errors.Wrapf(err, "int: failed to resolve TCP address")
	}

	shsAddr := netwrap.WrapAddr(plainAddr, secretstream.Addr{PubKey: remotePubKey})
	client, err := ssbClient.NewTCP(localKey, shsAddr,
		ssbClient.WithSHSAppKey(ctx.String("shscap")),
		ssbClient.WithContext(longctx))
	if err != nil {
		return nil, errors.Wrapf(err, "init: failed to connect to %s", shsAddr.String())
	}
	log.Log("init", "done")
	return client, nil
}

var callCmd = &cli.Command{
	Name:  "call",
	Usage: "make an dump* async call",
	UsageText: `SUPPORTS:
* whoami
* latestSequence
* getLatest
* get
* blobs.(has|want|rm|wants)
* gossip.(peers|add|connect)


see https://scuttlebot.io/apis/scuttlebot/ssb.html#createlogstream-source  for more

CAVEAT: only one argument...
`,
	Action: func(ctx *cli.Context) error {
		cmd := ctx.Args().Get(0)
		if cmd == "" {
			return errors.New("call: cmd can't be empty")
		}
		args := ctx.Args().Slice()
		v := strings.Split(cmd, ".")
		var sendArgs []interface{}
		if len(args) > 1 {
			sendArgs = make([]interface{}, len(args)-1)
			for i, v := range args[1:] {
				sendArgs[i] = v
			}
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		val, err := client.Async(longctx, reply, muxrpc.Method(v), sendArgs...) // TODO: args[1:]...
		if err != nil {
			return errors.Wrapf(err, "%s: call failed.", cmd)
		}
		log.Log("event", "call reply")
		jsonReply, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "%s: call failed.", cmd)
		}
		_, err = io.Copy(os.Stdout, bytes.NewReader(jsonReply))
		return errors.Wrapf(err, "%s: result copy failed.", cmd)
	},
}

var getCmd = &cli.Command{
	Name:  "get",
	Usage: "get a signle message from the database by key (%...)",
	Flags: []cli.Flag{&cli.BoolFlag{Name: "private"}},
	Action: func(ctx *cli.Context) error {
		key, err := refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("failed to validate message ref: %w", err)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		arg := struct {
			ID      *refs.MessageRef `json:"id"`
			Private bool             `json:"private"`
		}{key, ctx.Bool("private")}

		var val interface{}
		val, err = client.Async(longctx, val, muxrpc.Method{"get"}, arg)
		if err != nil {
			return err
		}
		log.Log("event", "get reply")
		jsonDrain(os.Stdout).Pour(longctx, val)
		return nil

	},
}

var connectCmd = &cli.Command{
	Name:  "connect",
	Usage: "connect to a remote peer",
	Action: func(ctx *cli.Context) error {
		to := ctx.Args().Get(0)
		if to == "" {
			return errors.New("connect: multiserv addr argument can't be empty")
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var val interface{}
		val, err = client.Async(longctx, val, muxrpc.Method{"ctrl", "connect"}, to)
		if err != nil {
			level.Warn(log).Log("event", "connect command failed", "err", err)

			// js fallback (our mux doesnt support authed namespaces)
			val, err = client.Async(longctx, val, muxrpc.Method{"gossip", "connect"}, to)
			if err != nil {
				return errors.Wrapf(err, "connect: async call failed.")
			}
		}
		log.Log("event", "connect reply")
		goon.Dump(val)
		return nil
	},
}

var blockCmd = &cli.Command{
	Name: "block",
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var blocked = make(map[*refs.FeedRef]bool)

		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			fr, err := refs.ParseFeedRef(sc.Text())
			if err != nil {
				return err
			}
			blocked[fr] = true
		}
		log.Log("blocking", len(blocked))

		var val interface{}
		val, err = client.Async(longctx, val, muxrpc.Method{"ctrl", "block"}, blocked)
		if err != nil {
			return err
		}
		log.Log("event", "block reply")
		goon.Dump(val)
		return nil
	},
}

var queryCmd = &cli.Command{
	Name:   "qry",
	Action: todo, //query,
}

var groupsCmd = &cli.Command{
	Name:  "groups",
	Usage: "group managment (create, invite, publishTo, etc.)",
	Subcommands: []*cli.Command{
		groupsCreateCmd,
		groupsInviteCmd,
		groupsPublishToCmd,
		groupsJoinCmd,
		/* TODO:
		groupsListCmd,
		groupsMembersCmd,
		*/
	},
}

var groupsCreateCmd = &cli.Command{
	Name:  "create",
	Usage: "create a new empty group",
	Action: func(ctx *cli.Context) error {
		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		name := ctx.Args().First()
		if name == "" {
			return fmt.Errorf("group name can't be empty")
		}

		var val interface{}
		val, err = client.Async(longctx, val, muxrpc.Method{"groups", "create"}, struct {
			Name string `json:"name"`
		}{name})
		if err != nil {
			return err
		}
		log.Log("event", "group created")
		goon.Dump(val)
		return nil
	},
}

var groupsInviteCmd = &cli.Command{
	Name:  "invite",
	Usage: "add people to a group",
	Action: func(ctx *cli.Context) error {
		args := ctx.Args()
		groupID, err := refs.ParseMessageRef(args.First())
		if err != nil {
			return fmt.Errorf("groupID needs to be a valid message ref: %w", err)
		}

		if groupID.Algo != refs.RefAlgoCloakedGroup {
			return fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo)
		}

		member, err := refs.ParseFeedRef(args.Get(1))
		if err != nil {
			return err
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		v, err := client.Async(longctx, reply, muxrpc.Method{"groups", "invite"}, groupID.Ref(), member.Ref())
		if err != nil {
			return errors.Wrapf(err, "invite call failed")
		}
		log.Log("event", "member added", "group", groupID.Ref(), "member", member.Ref())
		goon.Dump(v)
		return nil
	},
}

var groupsPublishToCmd = &cli.Command{
	Name:  "publishTo",
	Usage: "publish a handcrafted JSON blob to a group",
	Action: func(ctx *cli.Context) error {
		var content interface{}
		err := json.NewDecoder(os.Stdin).Decode(&content)
		if err != nil {
			return errors.Wrapf(err, "publish/raw: invalid json input from stdin")
		}

		groupID, err := refs.ParseMessageRef(ctx.Args().First())
		if err != nil {
			return fmt.Errorf("groupID needs to be a valid message ref: %w", err)
		}

		if groupID.Algo != refs.RefAlgoCloakedGroup {
			return fmt.Errorf("groupID needs to be a cloaked message ref, not %s", groupID.Algo)
		}

		client, err := newClient(ctx)
		if err != nil {
			return err
		}

		var reply interface{}
		v, err := client.Async(longctx, reply, muxrpc.Method{"groups", "publishTo"}, groupID.Ref(), content)
		if err != nil {
			return errors.Wrapf(err, "publish call failed.")
		}
		log.Log("event", "publishTo", "type", "raw")
		goon.Dump(v)
		return nil
	},
}

var groupsJoinCmd = &cli.Command{
	Name:   "join",
	Usage:  "manually join a group by adding the group key",
	Action: todo,
}
