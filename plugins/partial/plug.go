// SPDX-License-Identifier: MIT

package partial

import (
	"github.com/cryptix/go/logging"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog/roaring"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/muxmux"
	"go.cryptoscope.co/ssb/plugins/gossip"
)

type plugin struct {
	h muxrpc.Handler
}

const name = "partialReplication"

func (p plugin) Name() string {
	return name
}

func (p plugin) Method() muxrpc.Method {
	return muxrpc.Method{name}
}

func (p plugin) Handler() muxrpc.Handler {
	return p.h
}

// "partialReplication":{
// 	getFeed: 'source',
// 	getFeedReverse: 'source',
// 	getTangle: 'async',
// 	getMessagesOfType: 'source'
//   }

func New(log logging.Interface,
	fm *gossip.FeedManager,
	feeds, bytype, roots *roaring.MultiLog,
	rxlog margaret.Log) ssb.Plugin {
	rootHdlr := muxmux.New(log)

	// rootHdlr.RegisterAsync(muxrpc.Method{name, "getTangle"}, getTangle{
	// 	log:     log,
	// 	builder: b,
	// 	self:    self,
	// })

	rootHdlr.RegisterSource(muxrpc.Method{name, "getFeed"}, getFeedHandler{
		fm: fm,
	})

	rootHdlr.RegisterSource(muxrpc.Method{name, "getFeedReverse"}, getFeedReverseHandler{
		fm: fm,
	})

	rootHdlr.RegisterSource(muxrpc.Method{name, "getMessagesOfType"}, getMessagesOfTypeHandler{
		rxlog: rxlog,

		feeds:  feeds,
		bytype: bytype,
	})

	return plugin{
		h: &rootHdlr,
	}
}
