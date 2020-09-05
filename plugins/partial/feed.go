package partial

import (
	"context"

	"go.cryptoscope.co/ssb/plugins/gossip"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb/message"
)

//  https://ssbc.github.io/scuttlebutt-protocol-guide/#createHistoryStream

type getFeedHandler struct {
	fm *gossip.FeedManager
}

func (h getFeedHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk luigi.Sink) error {
	args := req.Args()

	if len(args) < 1 {
		return errors.New("ssb/message: not enough arguments, expecting feed id")

	}
	argMap, ok := args[0].(map[string]interface{})
	if !ok {
		err := errors.Errorf("ssb/message: not the right map - %T", args[0])
		return err

	}
	query, err := message.NewCreateHistArgsFromMap(argMap)
	if err != nil {
		return errors.Wrap(err, "bad request")
	}

	// query.Limit = 50
	// spew.Dump(query)
	err = h.fm.CreateStreamHistory(ctx, req.Stream, query)
	if err != nil {
		if luigi.IsEOS(err) {
			req.Stream.Close()
			return nil
		}
		return errors.Wrap(err, "createHistoryStream failed")
	}
	return nil
}

type getFeedReverseHandler struct {
	fm *gossip.FeedManager
}

func (h getFeedReverseHandler) HandleSource(ctx context.Context, req *muxrpc.Request, snk luigi.Sink) error {
	args := req.Args()

	if len(args) < 1 {
		return errors.New("ssb/message: not enough arguments, expecting feed id")

	}
	argMap, ok := args[0].(map[string]interface{})
	if !ok {
		err := errors.Errorf("ssb/message: not the right map - %T", args[0])
		return err

	}
	query, err := message.NewCreateHistArgsFromMap(argMap)
	if err != nil {
		return errors.Wrap(err, "bad request")
	}
	query.Reverse = true

	// query.Limit = 50
	// spew.Dump(query)
	err = h.fm.CreateStreamHistory(ctx, req.Stream, query)
	if err != nil {
		if luigi.IsEOS(err) {
			req.Stream.Close()
			return nil
		}
		return errors.Wrap(err, "createHistoryStream failed")
	}
	return nil
}
