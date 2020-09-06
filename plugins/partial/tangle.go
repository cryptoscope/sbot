package partial

import (
	"context"
	"encoding/json"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/mutil"
	"go.cryptoscope.co/ssb/internal/transform"

	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc"
	refs "go.mindeco.de/ssb-refs"
)

type getTangleHandler struct {
	rxlog margaret.Log

	get   ssb.Getter
	roots *roaring.MultiLog
}

func (h getTangleHandler) HandleAsync(ctx context.Context, req *muxrpc.Request) (interface{}, error) {
	var mrs []refs.MessageRef
	err := json.Unmarshal(req.RawArgs, &mrs)
	if err != nil {
		return nil, err
	}

	if len(mrs) != 1 {
		return nil, errors.Errorf("no args")
	}
	msg, err := h.get.Get(mrs[0])
	if err != nil {
		return nil, errors.Wrap(err, "getTangle: root message not found")
	}

	vals := []interface{}{
		msg.ValueContentJSON(),
	}

	threadLog, err := h.roots.Get(librarian.Addr(msg.Key().Hash))
	if err != nil {
		return nil, errors.Wrap(err, "getTangle: failed to load thread")
	}

	src, err := mutil.Indirect(h.rxlog, threadLog).Query()
	if err != nil {
		return nil, errors.Wrap(err, "getTangle: failed to qry tipe")
	}

	snk := luigi.NewSliceSink(&vals)
	err = luigi.Pump(ctx, transform.NewKeyValueWrapper(snk, false), src)
	if err != nil {
		return nil, errors.Wrap(err, "getTangle: failed to pump msgs")
	}
	return vals, nil
}
