// SPDX-License-Identifier: MIT

package get

import (
	"context"
	"log"

	"github.com/pkg/errors"
	"github.com/shurcooL/go-goon"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

type plugin struct {
	h muxrpc.Handler
}

func (p plugin) Name() string {
	return "get"
}

func (p plugin) Method() muxrpc.Method {
	return muxrpc.Method{"get"}
}

func (p plugin) Handler() muxrpc.Handler {
	return p.h
}

func New(g ssb.Getter, rl margaret.Log) ssb.Plugin {
	return plugin{
		h: handler{g: g, rl: rl},
	}
}

type handler struct {
	g  ssb.Getter
	rl margaret.Log
}

func (h handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args()) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}
	var (
		input  string
		parsed *refs.MessageRef
		err    error
	)
	switch v := req.Args()[0].(type) {
	case string:
		input = v
	case map[string]interface{}:
		goon.Dump(v)
		refV, ok := v["id"]
		if !ok {
			req.CloseWithError(errors.Errorf("invalid argument - missing 'id' in map"))
			return
		}
		input = refV.(string)
	default:
		req.CloseWithError(errors.Errorf("invalid argument type %T", req.Args()[0]))
		return
	}

	parsed, err = refs.ParseMessageRef(input)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to parse arguments"))
		return
	}

	msg, err := h.g.Get(*parsed)
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "failed to load message"))
		return
	}

	var kv refs.KeyValueRaw
	kv.Key_ = msg.Key()
	kv.Value = *msg.ValueContent()

	err = req.Return(ctx, kv)
	if err != nil {
		log.Printf("get(%s): failed? to return message: %s", parsed.Ref(), err)
	}
}
