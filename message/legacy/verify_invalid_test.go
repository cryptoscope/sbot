package legacy

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidFuzzed(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	var rawMsgs []json.RawMessage

	d, err := ioutil.ReadFile("./testdata-invalid.json")
	r.NoError(err, "failed to read file")

	err = json.Unmarshal(d, &rawMsgs)
	r.NoError(err)

	for i, m := range rawMsgs {
		r, d, err := Verify(m, nil)
		if !a.Error(err, "in msg %d\n%s:", i+1, string(m)) {
			continue
		}
		t.Log(i+1, "passed")
		a.Nil(r, i)
		a.Nil(d, i)
	}
}
