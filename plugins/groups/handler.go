// SPDX-License-Identifier: MIT

// Package groups supplies muxprc handlers for group managment.
package groups

import (
	"github.com/cryptix/go/logging"
	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/muxrpc"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/muxmux"
	"go.cryptoscope.co/ssb/private"
)

/*

  create: 'async',
  invite: 'async',
  publishTo: 'async',
*/

var (
	_      ssb.Plugin = plugin{} // compile-time type check
	method            = muxrpc.Method{"groups"}
)

func checkAndLog(log logging.Interface, err error) {
	if err != nil {
		level.Error(log).Log("err", err)
	}
}

func New(log logging.Interface, groups *private.Manager) ssb.Plugin {
	rootHdlr := muxmux.New(log)

	rootHdlr.RegisterAsync(append(method, "create"), create{
		log:    log,
		groups: groups,
	})

	rootHdlr.RegisterAsync(append(method, "publishTo"), publishTo{
		log:    log,
		groups: groups,
	})

	rootHdlr.RegisterAsync(append(method, "invite"), invite{
		log:    log,
		groups: groups,
	})

	return plugin{
		h:   &rootHdlr,
		log: log,
	}
}

type plugin struct {
	h   muxrpc.Handler
	log logging.Interface
}

func (plugin) Name() string              { return method[0] }
func (plugin) Method() muxrpc.Method     { return method }
func (p plugin) Handler() muxrpc.Handler { return p.h }
