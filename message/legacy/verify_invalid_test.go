package legacy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	refs "go.mindeco.de/ssb-refs"
)

func TestInvalidFuzzed(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	type state struct{}

	type testCase struct {
		State   *state
		HMACKey b64str `json:"hmacKey"`

		Message json.RawMessage

		Error *string
		Valid bool
		ID    refs.MessageRef
	}

	var rawMsgs []testCase

	d, err := ioutil.ReadFile("./testdata-fuzzed.json")
	r.NoError(err, "failed to read file")

	err = json.Unmarshal(d, &rawMsgs)
	r.NoError(err)

	for i, m := range rawMsgs {
		r, d, err := Verify(m.Message, nil)

		if m.Valid {
			if !a.NoError(err, "in msg %d", i+1) {
				continue
			}
			a.NotNil(r, i)
			a.NotNil(d, i)
		} else {
			if !a.Error(err, "in msg %d", i+1) {
				t.Logf("error: %s", *m.Error)
				continue
			}
			a.Nil(r, i)
			a.Nil(d, i)
		}
	}
}

type b64str []byte

func (s *b64str) UnmarshalJSON(data []byte) error {
	var strdata string
	err := json.Unmarshal(data, &strdata)
	if err != nil {
		if bytes.Equal(data, []byte("null")) {
			*s = nil
			return nil
		}
		return fmt.Errorf("b64str: json decode of string failed: %w", err)
	}
	decoded := make([]byte, len(strdata)) // will be shorter
	n, err := base64.StdEncoding.Decode(decoded, []byte(strdata))
	if err != nil {
		return fmt.Errorf("invalid base64 string (%q): %w", strdata, err)
	}

	*s = decoded[:n]
	return nil
}
