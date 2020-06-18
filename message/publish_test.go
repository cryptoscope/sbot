// SPDX-License-Identifier: MIT

package message

import (
	"context"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

func TestSignMessages(t *testing.T) {
	tctx := context.TODO()
	r := require.New(t)
	a := assert.New(t)

	rpath := filepath.Join("testrun", t.Name())
	os.RemoveAll(rpath)

	testRepo := repo.New(rpath)
	rl, err := repo.OpenLog(testRepo)

	r.NoError(err, "failed to open root log")
	seq, err := rl.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(-1), seq, "not empty")

	userFeeds, userFeedsServe, err := multilogs.OpenUserFeeds(testRepo)
	r.NoError(err, "failed to get user feeds multilog")

	killServe, cancel := context.WithCancel(tctx)
	defer cancel()
	errc := make(chan error)
	go func() {
		err := userFeedsServe(killServe, rl, true)
		errc <- errors.Wrap(err, "failed to pump log into userfeeds multilog")
	}()

	staticRand := rand.New(rand.NewSource(42))
	testAuthor, err := ssb.NewKeyPair(staticRand)
	r.NoError(err)

	authorLog, err := userFeeds.Get(testAuthor.Id.StoredAddr())
	r.NoError(err)

	w, err := OpenPublishLog(rl, userFeeds, testAuthor)
	r.NoError(err)

	var tmsgs = []interface{}{
		map[string]interface{}{
			"type":  "about",
			"about": testAuthor.Id.Ref(),
			"name":  "test user",
		},
		map[string]interface{}{
			"type":      "contact",
			"contact":   "@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519",
			"following": true,
		},
		map[string]interface{}{
			"type": "text",
			"text": `# hello world!`,
		},
	}
	for i, msg := range tmsgs {
		newSeq, err := w.Append(msg)
		r.NoError(err, "failed to pour test message %d", i)
		currSeq, err := authorLog.Seq().Value()
		r.NoError(err, "failed to get log seq")
		r.Equal(margaret.BaseSeq(i), newSeq, "advanced")
		r.Equal(newSeq, currSeq, "same new sequences")
	}

	latest, err := authorLog.Seq().Value()
	r.NoError(err, "failed to get log seq")
	r.Equal(margaret.BaseSeq(2), latest, "not empty")

	for i := 0; i < len(tmsgs); i++ {
		rootSeq, err := authorLog.Get(margaret.BaseSeq(i))
		r.NoError(err)
		storedV, err := rl.Get(rootSeq.(margaret.Seq))
		r.NoError(err)
		storedMsg, ok := storedV.(refs.Message)
		r.True(ok)
		t.Logf("msg:%d\n%s", i, storedMsg.ContentBytes())
		a.NotNil(storedMsg.Key(), "msg:%d - key", i)
		if i != 0 {
			// a.NotNil(storedMsg.Previous, "msg:%d - previous", i)
		} else {
			// a.Nil(storedMsg.Previous)
		}
		// a.NotNil(storedMsg.Raw, "msg:%d - raw", i)
		// a.Contains(string(storedMsg.Raw), `"signature": "`)
		// a.Contains(string(storedMsg.Raw), fmt.Sprintf(`"sequence": %d`, i+1))

	}
}
