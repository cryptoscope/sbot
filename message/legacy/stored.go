// SPDX-License-Identifier: MIT

package legacy

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cryptix/go/encodedTime"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/margaret"
)

// OldStoredMessage is only available to ease migration from old, pre-multimsg formats
type OldStoredMessage struct {
	Author    *refs.FeedRef    // @... pubkey
	Previous  *refs.MessageRef // %... message hashsha
	Key       *refs.MessageRef // %... message hashsha
	Sequence  margaret.BaseSeq
	Timestamp time.Time
	Raw       []byte // the original message for gossiping see ssb.EncodePreserveOrdering for why
}

func (sm OldStoredMessage) String() string {
	s := fmt.Sprintf("msg(%s:%d) %s", sm.Author.Ref(), sm.Sequence, sm.Key.Ref())
	b, _ := EncodePreserveOrder(sm.Raw)
	s += "\n"
	s += string(b)
	return s
}

// really dislike the underlines but they are there to implement the message interface more easily

type StoredMessage struct {
	Author_    *refs.FeedRef    // @... pubkey
	Previous_  *refs.MessageRef // %... message hashsha
	Key_       *refs.MessageRef // %... message hashsha
	Sequence_  margaret.BaseSeq
	Timestamp_ time.Time
	Raw_       []byte // the original message for gossiping see ssb.EncodePreserveOrdering for why

	// TODO: consider lazy decoding approach from gabbygrove to reduce storage overhead
}

// could use this to unexport fields, would require lots of constructors though
// func (sm StoredMessage) MarshalBinary() ([]byte, error) {
// }
// func (sm *StoredMessage) UnmarshalBinary(data []byte) error {
// }

func (sm StoredMessage) String() string {
	s := fmt.Sprintf("msg(%s:%d) %s", sm.Author_.Ref(), sm.Sequence_, sm.Key_.Ref())
	b, _ := EncodePreserveOrder(sm.Raw_)
	s += "\n"
	s += string(b)
	return s
}

var _ refs.Message = (*StoredMessage)(nil)

func (sm StoredMessage) Seq() int64 {
	return sm.Sequence_.Seq()
}

func (sm StoredMessage) Key() *refs.MessageRef {
	return sm.Key_
}

func (sm StoredMessage) Author() *refs.FeedRef {
	return sm.Author_
}

func (sm StoredMessage) Previous() *refs.MessageRef {
	return sm.Previous_
}

func (sm StoredMessage) Claimed() time.Time {
	vc := sm.ValueContent()
	return time.Time(vc.Timestamp)
}

func (sm StoredMessage) Received() time.Time {
	return sm.Timestamp_
}

func (sm StoredMessage) ContentBytes() []byte {
	var c struct {
		Content json.RawMessage `json:"content"`
	}
	err := json.Unmarshal(sm.Raw_, &c)
	if err != nil {
		log.Println("warning: Content of storedMessage failed:", err)
		return nil
	}
	return c.Content
}

func (sm StoredMessage) ValueContent() *refs.Value {
	var msg refs.Value
	msg.Previous = sm.Previous_
	msg.Author = *sm.Author_
	msg.Sequence = sm.Sequence_
	msg.Hash = "sha256"
	var cs struct {
		Timestamp encodedTime.Millisecs `json:"timestamp"`
		Content   json.RawMessage       `json:"content"`
		Signature string                `json:"signature"`
	}
	err := json.Unmarshal(sm.Raw_, &cs)
	if err != nil {
		log.Println("warning: Content of storedMessage failed:", err)
		return nil
	}
	msg.Content = cs.Content
	msg.Signature = cs.Signature
	msg.Timestamp = cs.Timestamp
	return &msg
}

func (sm StoredMessage) ValueContentJSON() json.RawMessage {
	return sm.Raw_
}
