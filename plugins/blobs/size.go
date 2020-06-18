// SPDX-License-Identifier: MIT

package blobs

import (
	"context"
	"encoding/json"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

type sizeHandler struct {
	bs  ssb.BlobStore
	log logging.Interface
}

func (sizeHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h sizeHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: push manifest check into muxrpc
	if req.Type == "" {
		req.Type = "async"
	}

	var blobs []*refs.BlobRef
	err := json.Unmarshal(req.RawArgs, &blobs)
	if err != nil {
		req.Stream.CloseWithError(errors.Wrap(err, "error parsing blob reference"))
		return
	}
	if len(blobs) != 1 {
		req.Stream.CloseWithError(errors.Errorf("bad request", len(blobs)))
		return
	}
	sz, err := h.bs.Size(blobs[0])
	if err != nil {
		err = errors.Wrap(err, "error looking up blob")
		err = req.Stream.CloseWithError(err)
		checkAndLog(h.log, err)
		return
	}

	err = req.Return(ctx, sz)
	checkAndLog(h.log, errors.Wrap(err, "error returning value"))
}
