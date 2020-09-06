package network

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/secretstream"
)

func websockHandler(n *node) http.HandlerFunc {
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024 * 4,
		WriteBufferSize: 1024 * 4,
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
		EnableCompression: false,
	}
	return func(w http.ResponseWriter, req *http.Request) {
		fmt.Println("ssb-ws:", req.URL.String())
		remoteAddr, err := net.ResolveTCPAddr("tcp", req.RemoteAddr)
		if err != nil {
			n.log.Log("warning", "failed wrap", "err", err, "remote", remoteAddr)
			return
		}
		wsConn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			n.log.Log("warning", "failed wrap", "err", err, "remote", remoteAddr)
			return
		}

		var wc net.Conn
		wc = &wrappedConn{
			remote: netwrap.WrapAddr(remoteAddr, secretstream.Addr{
				PubKey: n.opts.KeyPair.Id.ID,
			}),
			local: &net.TCPAddr{
				IP:   nil,
				Port: 8989,
			},
			wsc: wsConn,
		}

		level.Info(n.log).Log("event", "new ws conn", "r", remoteAddr)

		// comment out this block to get `noauth` instead of `shs`
		cw := n.secretServer.ConnWrapper()
		wc, err = cw(wc)
		if err != nil {
			level.Error(n.log).Log("warning", "failed to crypt", "err", err, "remote", remoteAddr)
			wsConn.Close()
			return
		}

		// debugging copy of all muxrpc frames
		// can be handy for reversing applications
		// wrapped, err := debug.WrapDump("webmux", cryptoConn)
		// if err != nil {
		// 	level.Error(n.log).Log("warning", "failed wrap", "err", err, "remote", remoteAddr)
		// 	wsConn.Close()
		// 	return
		// }

		pkr := muxrpc.NewPacker(wc)

		h, err := n.opts.MakeHandler(wc)
		if err != nil {
			err = errors.Wrap(err, "unix sock make handler")
			level.Error(n.log).Log("warn", err)
			wsConn.Close()
			return
		}

		level.Debug(n.log).Log("event", "ws handler made - serving")

		edp := muxrpc.HandleWithRemote(pkr, h, wc.RemoteAddr())

		srv := edp.(muxrpc.Server)
		// TODO: bundle root and connection context
		if err := srv.Serve(req.Context()); err != nil {
			level.Error(n.log).Log("conn", "serve exited", "err", err, "peer", remoteAddr)
		}
		wsConn.Close()
	}
}

type wrappedConn struct {
	remote net.Addr
	local  net.Addr

	r   io.Reader
	wsc *websocket.Conn
}

func (conn *wrappedConn) Read(data []byte) (int, error) {
	if conn.r == nil {
		if err := conn.renewReader(); err != nil {
			return -1, err
		}

	}
	n, err := conn.r.Read(data)
	if err == io.EOF {
		if err := conn.renewReader(); err != nil {
			return -1, err
		}
		return conn.Read(data)
	}

	return n, err
}

func (wc *wrappedConn) renewReader() error {
	mt, r, err := wc.wsc.NextReader()
	if err != nil {
		return errors.Wrap(err, "wsConn: failed to get reader")
	}

	if mt != websocket.BinaryMessage {
		return errors.Errorf("wsConn: not binary message: %v", mt)

	}
	wc.r = r
	return nil
}

func (conn wrappedConn) Write(data []byte) (int, error) {
	writeCloser, err := conn.wsc.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return -1, errors.Wrap(err, "wsConn: failed to create Reader")
	}

	n, err := io.Copy(writeCloser, bytes.NewReader(data))
	if err != nil {
		return -1, errors.Wrap(err, "wsConn: failed to copy data")
	}
	return int(n), writeCloser.Close()
}

func (conn wrappedConn) Close() error {
	return conn.wsc.Close()
}

func (c wrappedConn) LocalAddr() net.Addr  { return c.local }
func (c wrappedConn) RemoteAddr() net.Addr { return c.remote }
func (c wrappedConn) SetDeadline(t time.Time) error {
	return nil // c.conn.SetDeadline(t)
}
func (c wrappedConn) SetReadDeadline(t time.Time) error {
	return nil // c.conn.SetReadDeadline(t)
}
func (c wrappedConn) SetWriteDeadline(t time.Time) error {
	return nil // c.conn.SetWriteDeadline(t)
}
