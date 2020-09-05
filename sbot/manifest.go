package sbot

import (
	"context"
	"encoding/json"

	"go.cryptoscope.co/muxrpc"
)

type namedPlugin struct {
	h    muxrpc.Handler
	name string
}

func (np namedPlugin) Name() string { return np.name }

func (np namedPlugin) Method() muxrpc.Method {
	return muxrpc.Method{np.name}
}

func (np namedPlugin) Handler() muxrpc.Handler {
	return np.h
}

type manifestHandler string

func (manifestHandler) HandleConnect(context.Context, muxrpc.Endpoint) {}

func (h manifestHandler) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	req.Return(ctx, json.RawMessage(h))
}

// this is a very simple hardcoded manifest.json dump which oasis' ssb-client expects to do it's magic.
const manifestBlob = `
{
	"auth": "async",
	"address": "sync",
	"manifest": "sync",

	"multiserverNet": {},
	"get": "async",
	"createFeedStream": "source",
	"createUserStream": "source",
	"createWriteStream": "sink",
	"links": "source",

	"add": "async",

	"getLatest": "async",
	"latest": "source",
	"latestSequence": "async",

	"createSequenceStream": "source",
	"createLogStream": "source",
	"messagesByType": "source",
	"createHistoryStream": "source",

	"partialReplication":{
	 	"getFeed": 'source',
	 	"getFeedReverse": 'source',
	 	"getTangle": 'async',
	 	"getMessagesOfType": 'source'
	},
		

	"tangles": "source",
    "names": {
        "get": "async",
        "getImageFor": "async",
        "getSignifier": "async"
    },

	"friends": {
	  "isFollowing": "async"
	},

	"publish": "async",
	"whoami": "sync",
	"status": "sync",
	"gossip": {
	  "connect": "async",
	  "ping": "duplex"
	},
	"replicate": {
	  "upto": "source"
	},

	"blobs": {
	  "get": "source",

	  "add": "sink",
	  "rm": "async",
	  "ls": "source",
	  "has": "async",
	  "size": "async",

	  "want": "async",

	  "createWants": "source"
	}
  }
  `
