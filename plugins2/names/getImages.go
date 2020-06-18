// SPDX-License-Identifier: MIT

package names

import (
	"context"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	refs "go.mindeco.de/ssb-refs"
)

type hImagesFor struct {
	as  aboutStore
	log logging.Interface
}

func (hImagesFor) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h hImagesFor) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	ref, err := parseFeedRefFromArgs(req)
	if err != nil {
		checkAndLog(h.log, err)
		return
	}

	ai, err := h.as.CollectedFor(ref)
	if err != nil {
		err = req.Stream.CloseWithError(errors.Errorf("do not have about for: %s", ref.Ref()))
		checkAndLog(h.log, errors.Wrap(err, "error closing stream with error"))
		return
	}
	if ai.Image.Chosen != "" {
		err = req.Return(ctx, ai.Image.Chosen)
		checkAndLog(h.log, errors.Wrap(err, "error returning chosen value"))
		return
	}
	var hottest string
	var most = 0
	for v, cnt := range ai.Image.Prescribed {
		if most > cnt {
			most = cnt
			hottest = v
		}
	}
	err = req.Return(ctx, hottest)
	checkAndLog(h.log, errors.Wrap(err, "error returning chosen value"))
	return
}

func checkAndLog(log logging.Interface, err error) {
	if err != nil {
		log.Log("handlerErr", err)
	}
}

func parseFeedRefFromArgs(req *muxrpc.Request) (*refs.FeedRef, error) {
	args := req.Args()
	if len(args) != 1 {
		return nil, errors.Errorf("not enough args")
	}

	var refStr string
	switch arg := args[0].(type) {
	case string:
		refStr = arg
	case map[string]interface{}:
		refStr, _ = arg["id"].(string)
	}

	ref, err := refs.ParseFeedRef(refStr)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing feed reference")
	}

	return ref, nil
}
