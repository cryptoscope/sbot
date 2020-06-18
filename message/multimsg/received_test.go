package multimsg

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret/offset2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"

	gabbygrove "go.mindeco.de/ssb-gabbygrove"
	refs "go.mindeco.de/ssb-refs"
)

// fakeNow can be used as a stub for time.Now()
// it unsets itself each call so set the next field before calling it
type fakeNow struct{ next time.Time }

func (fn *fakeNow) Now() time.Time {
	nxt := fn.next
	fn.next = time.Unix(13*60*60+37*60, 0)
	return nxt
}

func TestReceivedSet(t *testing.T) {
	r := require.New(t)

	tPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tPath)

	log, err := offset2.Open(tPath, MargaretCodec{})
	r.NoError(err)

	wl := NewWrappedLog(log)
	fn := fakeNow{}
	wl.receivedNow = fn.Now

	bobsKey, err := ssb.NewKeyPair(nil)
	r.NoError(err)

	// quirky way to make a refs.Message
	var lm legacy.LegacyMessage
	lm.Hash = "sha256"
	lm.Author = bobsKey.Id.Ref()
	lm.Previous = nil
	lm.Sequence = 666

	newMsg := &legacy.StoredMessage{
		Author_:   bobsKey.Id,
		Previous_: lm.Previous,
		Sequence_: lm.Sequence,
		Raw_:      []byte(`"fakemsg"`),
	}

	fn.next = time.Unix(23, 0)
	seq, err := wl.Append(newMsg)
	r.NoError(err)
	r.NotNil(seq)

	// retreive it
	gotV, err := wl.Get(seq)
	r.NoError(err)

	gotMsg, ok := gotV.(refs.Message)
	r.True(ok, "got %T", gotV)

	// check the received
	rxt := gotMsg.Received()
	r.NotNil(rxt)
	r.EqualValues(23, rxt.Unix(), "time: %s", rxt)

	// a gabby message
	bobsKey.Id.Algo = ssb.RefAlgoFeedGabby

	enc := gabbygrove.NewEncoder(bobsKey)

	tr, ref, err := enc.Encode(1, nil, "hello, world")
	r.NoError(err)
	r.NotNil(ref)

	//fn.next = time.Unix(42, 0)
	ggSeq, err := wl.Append(tr)
	r.NoError(err)
	r.NotNil(ggSeq)

	gotV, err = wl.Get(ggSeq)
	r.NoError(err)

	gotMsg, ok = gotV.(refs.Message)
	r.True(ok, "got %T", gotV)

	rxt = gotMsg.Received()
	r.NotNil(rxt)
	r.EqualValues(42, rxt.Unix(), "time: %s", rxt)

	r.NoError(log.Close())
}
