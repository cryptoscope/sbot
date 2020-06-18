// SPDX-License-Identifier: MIT

package peerinvites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cryptix/go/logging"
	"github.com/dgraph-io/badger"
	kitlog "github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	libbadger "go.cryptoscope.co/librarian/badger"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/muxrpc"
	"go.cryptoscope.co/ssb/message/legacy"
	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/nacl/auth"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

type Plugin struct {
	tl multilog.MultiLog
	rl margaret.Log

	logger logging.Interface

	h handler
}

func (p Plugin) Name() string {
	return "get"
}

func (p Plugin) Method() muxrpc.Method {
	return muxrpc.Method{"peerInvites"}
}

func (p Plugin) Handler() muxrpc.Handler {
	return p.h
}

const FolderNameInvites = "peerInvites"

func (p *Plugin) OpenIndex(r repo.Interface) (librarian.Index, repo.ServeFunc, error) {
	db, sinkIdx, serve, err := repo.OpenBadgerIndex(r, FolderNameInvites, p.updateIndex)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting index")
	}
	nextServe := func(ctx context.Context, log margaret.Log, live bool) error {
		err := serve(ctx, log, live)
		if err != nil {
			return err
		}
		return db.Close()
	}
	return sinkIdx, nextServe, nil
}

func (p *Plugin) updateIndex(db *badger.DB) librarian.SinkIndex {
	p.h.state = libbadger.NewIndex(db, true)

	idxSink := librarian.NewSinkIndex(func(ctx context.Context, seq margaret.Seq, val interface{}, idx librarian.SetterIndex) error {
		msg, ok := val.(refs.Message)
		if !ok {
			return fmt.Errorf("unexpeced stored message type: %T", val)
		}
		var msgType struct {
			Type string `json:"type"`
		}
		err := json.Unmarshal(msg.ContentBytes(), &msgType)
		if err != nil {
			// p.logger.Log("skipped", msg.Key().Ref(), "err", err)
			return nil
		}

		// TODO: multitypes query!?! :D
		switch msgType.Type {
		case "peer-invite":
			err = p.indexNewInvite(ctx, msg)
			p.logger.Log("newInvite", err, "msg", msg.Key().Ref())
			return err
		case "peer-invite/confirm":
			err := p.indexConfirm(ctx, msg)
			p.logger.Log("confirmed", err)
			return err
		default:
			// p.logger.Log("skipped", msg.Key().Ref(), "why", "wrong type", "type", msgType.Type)
			return nil // skip
		}
	}, p.h.state)
	return idxSink
}

func (p *Plugin) indexNewInvite(ctx context.Context, msg refs.Message) error {

	var invCore struct {
		Invite *refs.FeedRef `json:"invite"`
		Host   *refs.FeedRef `json:"host"`
	}
	err := json.Unmarshal(msg.ContentBytes(), &invCore)
	if err != nil {
		return err
	}

	if invCore.Invite == nil {
		return fmt.Errorf("invalid invite")
	}
	guestRef := invCore.Invite.Ref()
	idxAddr := librarian.Addr(guestRef)

	obv, err := p.h.state.Get(ctx, idxAddr)
	if err != nil {
		return errors.Wrap(err, "idx get failed")
	}

	obvV, err := obv.Value()
	if err != nil {
		return errors.Wrap(err, "idx value failed")
	}

	switch v := obvV.(type) {
	case bool:
		if v {
			return errors.Errorf("invites: guest ID already in use")
		}
		// ok, reuse

	case librarian.UnsetValue:
		// ok, fresh guest key

	default:
		return fmt.Errorf("unhandled index type for new invite message: %T", obvV)
	}
	p.logger.Log("msg", "got invite", "author", msg.Author().Ref(), "guest", guestRef)
	return p.h.state.Set(ctx, idxAddr, true)
}

func (p *Plugin) indexConfirm(ctx context.Context, msg refs.Message) error {
	var invConfirm struct {
		Embed struct {
			Content acceptContent `json:"content"`
		} `json:"embed"`
	}
	if err := json.Unmarshal(msg.ContentBytes(), &invConfirm); err != nil {
		return err
	}
	accptMsg := invConfirm.Embed.Content

	if accptMsg.Receipt == nil {
		return fmt.Errorf("invalid recipt on confirm msg")
	}

	reciept, err := p.h.g.Get(*accptMsg.Receipt)
	if err != nil {
		return err
	}

	var invCore struct {
		Invite *refs.FeedRef `json:"invite"`
		Host   *refs.FeedRef `json:"host"`
	}

	if err := json.Unmarshal(reciept.ContentBytes(), &invCore); err != nil {
		return err
	}

	idxAddr := librarian.Addr(invCore.Invite.Ref())
	p.logger.Log("msg", "invite confirmed", "author", msg.Author().Ref(), "guest", idxAddr)
	return p.h.state.Set(ctx, idxAddr, false)
}

func (p *Plugin) Authorize(to *refs.FeedRef) error {
	obv, err := p.h.state.Get(context.Background(), librarian.Addr(to.Ref()))
	if err != nil {
		return errors.Wrap(err, "idx state get failed")
	}
	v, err := obv.Value()
	if err != nil {
		return errors.Wrap(err, "idx value failed")
	}
	if valid, ok := v.(bool); ok && valid {
		p.logger.Log("authorized", "auth", "to", to.Ref())
		return nil
	}
	return errors.New("not for us")
}

var (
	_ ssb.Plugin     = (*Plugin)(nil)
	_ ssb.Authorizer = (*Plugin)(nil)
)

func New(logger logging.Interface, g ssb.Getter, typeLog multilog.MultiLog, rootLog margaret.Log, publish ssb.Publisher) *Plugin {

	p := Plugin{
		logger: logger,

		tl: typeLog,
		rl: rootLog,

		h: handler{
			logger: logger,

			g:   g,
			tl:  typeLog,
			rl:  rootLog,
			pub: publish,
		},
	}

	return &p
}

type handler struct {
	logger logging.Interface

	state librarian.SeqSetterIndex

	g ssb.Getter

	tl multilog.MultiLog
	rl margaret.Log

	pub ssb.Publisher
}

func (h handler) HandleConnect(ctx context.Context, e muxrpc.Endpoint) {}

func (h handler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	if len(req.Args()) < 1 {
		req.CloseWithError(errors.Errorf("invalid arguments"))
		return
	}

	guestRef, err := ssb.GetFeedRefFromAddr(edp.Remote())
	if err != nil {
		req.CloseWithError(errors.Wrap(err, "no guest ref!?"))
		return
	}

	hlog := kitlog.With(h.logger, "method", req.Method.String())
	// hlog.Log("peerInvites", "called")
	errLog := level.Error(hlog)
	switch req.Method.String() {
	case "peerInvites.willReplicate":
		// addtional graph dist check?
		// we know they are in range since the default graph check
		// but could be played with different values for each..
		req.Return(ctx, true)
		// req.CloseWithError(fmt.Errorf("sorry"))
	case "peerInvites.getInvite":

		ref, err := refs.ParseMessageRef(req.Args()[0].(string))
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "failed to parse arguments"))
			return
		}
		msg, err := h.g.Get(*ref)
		if err != nil {
			err = errors.Wrap(err, "failed to get referenced message")
			errLog.Log("err", err)
			req.CloseWithError(err)
			return
		}

		// invite data matches
		var invCore struct {
			Invite *refs.FeedRef `json:"invite"`
			Host   *refs.FeedRef `json:"host"`
		}
		err = json.Unmarshal(msg.ContentBytes(), &invCore)
		if err != nil {
			// spew.Dump(msg.ContentBytes())
			err = errors.Wrap(err, "failed to decode stored message")
			errLog.Log("err", err)
			req.CloseWithError(err)
			return
		}

		if !bytes.Equal(invCore.Invite.ID, guestRef.ID) {
			err = errors.Errorf("not your invite")
			errLog.Log("err", err)
			req.CloseWithError(err)
			return
		}

		err = req.Return(ctx, json.RawMessage(msg.ValueContentJSON()))
		if err != nil {
			errLog.Log("msg", "failed to return message", "err", err)
			return
		}

	case "peerInvites.confirm":

		// shady way to check that its an array with 1 elem
		msgArg := bytes.TrimSuffix([]byte(req.RawArgs), []byte("]"))
		msgArg = bytes.TrimPrefix(msgArg, []byte("["))

		accept, err := verifyAcceptMessage(msgArg, guestRef)
		if err != nil {
			err = errors.Wrap(err, "failed to validate accept msg")
			errLog.Log("err", err)
			req.CloseWithError(err)
			return
		}
		fmt.Fprintln(os.Stderr, string(msgArg))
		fmt.Fprintf(os.Stderr, "%+v\n", accept)
		ref, err := h.pub.Publish(struct {
			Type  string          `json:"type"`
			Embed json.RawMessage `json:"embed"`
		}{"peer-invite/confirm", msgArg})
		if err != nil {
			errors.Wrap(err, "failed to publish confirm message")
			errLog.Log("err", err)
			req.CloseWithError(err)
			return
		}

		msg, err := h.g.Get(*ref)
		if err != nil {
			errors.Wrap(err, "failed to load published confirm message")
			errLog.Log("err", err)
			req.CloseWithError(err)
			return
		}

		// legacy contact message
		// confirm should implicate alice<>bob are friends
		_, err = h.pub.Publish(struct {
			Type       string           `json:"type"`
			Contact    *refs.FeedRef    `json:"contact"`
			Following  bool             `json:"following"`
			AutoFollow bool             `json:"auto"`
			Receipt    *refs.MessageRef `json:"peerReceipt"`
		}{"contact", accept.ID, true, true, accept.Receipt})
		if err != nil {
			req.CloseWithError(errors.Wrap(err, "failed to publish confirm message"))
			return
		}

		err = req.Return(ctx, json.RawMessage(msg.ContentBytes()))
	default:
		req.CloseWithError(fmt.Errorf("unknown method"))
	}
	hlog.Log("peerInvites", "done")
}

//  from 2.0: hash("peer-invites")
var peerCap = [32]byte{29, 61, 48, 33, 139, 164, 220, 229, 156, 216, 91, 90, 9, 241, 205, 157, 169, 21, 235, 200, 210, 25, 26, 227, 68, 195, 253, 42, 139, 59, 33, 7}

func verifyAcceptMessage(raw []byte, guestID *refs.FeedRef) (*acceptContent, error) {
	var rawContent struct {
		Content json.RawMessage
	}
	err := json.Unmarshal(raw, &rawContent)
	if err != nil {
		return nil, errors.Wrap(err, "unwrap content for verify failed")
	}

	// fmt.Fprintln(os.Stderr, "msg:", string(rawContent.Content))

	// can verify the invite message
	enc, err := legacy.EncodePreserveOrder(rawContent.Content)
	if err != nil {
		return nil, err
	}
	invmsgWoSig, sig, err := legacy.ExtractSignature(enc)
	if err != nil {
		return nil, err
	}

	mac := auth.Sum(invmsgWoSig, &peerCap)
	err = sig.Verify(mac[:], guestID)
	if err != nil {
		return nil, err
	}

	var inviteAccept struct {
		Author  *refs.FeedRef `json:"author"`
		Content acceptContent
	}

	if err := json.Unmarshal(raw, &inviteAccept); err != nil {
		return nil, errors.Wrap(err, "unwrap content for sanatize failed")
	}

	if inviteAccept.Content.Type != "peer-invite/accept" {
		return nil, errors.Errorf("invalid type on accept message")
	}

	if !bytes.Equal(inviteAccept.Author.ID, inviteAccept.Content.ID.ID) {
		return nil, errors.Errorf("invte is not for the right guest")
	}

	return &inviteAccept.Content, nil
}

type acceptContent struct {
	Type    string           `json:"type"`
	Receipt *refs.MessageRef `json:"receipt"`
	ID      *refs.FeedRef    `json:"id"`
	// Key     string          `json:"key"` only needed for reveal
}
