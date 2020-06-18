// SPDX-License-Identifier: MIT

// +build ignore

// TODO: make testdata fetchable

package multilogs

import (
	"context"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.cryptoscope.co/margaret/multilog"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret"
)

func TestStaticRepos(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)
	_, err := os.Stat("testdata")
	if os.IsNotExist(err) {
		_, err = os.Stat("testdata.tar.zst")
		r.NoError(err, "did not find testdata archive, please download...")
	} else {
		os.RemoveAll("testdata")
	}

	shasum := exec.Command("sha512sum", "-c", "testdata.tar.zst.shasum")
	out, err := shasum.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		r.NoError(err)
	}

	untar := exec.Command("tar", "xf", "testdata.tar.zst")
	out, err = untar.CombinedOutput()
	if err != nil {
		t.Log(string(out))
		r.NoError(err)
	}

	type tFeedSet map[string]int
	type tcase struct {
		Name      string
		Length    int
		HeadCount tFeedSet
	}
	cases := []tcase{
		{
			Name:   "small",
			Length: 10000,
			HeadCount: tFeedSet{
				"@C/zyw6xOd/QIcTLuZKZcrMoMmTwMfQs0u0rWEhNksgw=.ed25519": 30,
				"@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519": 417,
				"@HDFD0DqC4yxMItsq9jrbj3ZGgTzThbjCuApdFpWw6v8=.ed25519": 11,
				"@HEqy940T6uB+T+d9Jaa58aNfRzLx9eRWqkZljBmnkmk=.ed25519": 135,
				"@LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=.ed25519": 13,
				"@MRiJ+CvDnD9ZjqunY1oy6tsk0IdbMDC4Q3tTC8riS3s=.ed25519": 1203,
				"@fBS90Djngwl/SlCh/20G7piSC064Qz2hBBxbfnbyM+Y=.ed25519": 2244,
				"@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519": 4416,
				"@qv10rF4IsmxRZb7g5ekJ33EakYBpdrmV/vtP1ij5BS4=.ed25519": 981,
				"@wKaaftUxisV3GA3zuk2ena5krzJgB/6HwvgOKKIR/jQ=.ed25519": 8,
				"@xxZeuBUKIXYZZ7y+CIHUuy0NOFXz2MhUBkHemr86S3M=.ed25519": 526,
				"@1nXNP4vDRn2Y7LsPA9/VTRnFOOKjsjf2ioeA2wW50KQ=.ed25519": 4,
			},
		},
		{
			Name:   "medium",
			Length: 50000,
			HeadCount: tFeedSet{
				"@C/zyw6xOd/QIcTLuZKZcrMoMmTwMfQs0u0rWEhNksgw=.ed25519": 30,
				"@DJVbfRTEhC3cH1zU5/BAx6MgKIX1fkkytaRziA10suw=.ed25519": 306,
				"@D26sJ/Seyc4WBcpZDi4PYcqEg+2nUb7WoTQg9NknDyg=.ed25519": 1875,
				"@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519": 8111,
				"@HDFD0DqC4yxMItsq9jrbj3ZGgTzThbjCuApdFpWw6v8=.ed25519": 11,
				"@HEqy940T6uB+T+d9Jaa58aNfRzLx9eRWqkZljBmnkmk=.ed25519": 8347,
				"@LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=.ed25519": 13,
				"@MRiJ+CvDnD9ZjqunY1oy6tsk0IdbMDC4Q3tTC8riS3s=.ed25519": 1203,
				"@OF30xmIi8yCA6nLuu8UtCxy+LAzRJ3yzH/s6Ia6ejGs=.ed25519": 138,
				"@Q6jeOaoJOdFq8/3oFaOYC6bKhaVCD3IIdpjG+7Fab2M=.ed25519": 4468,
				"@arDWwU9A6/JxjaCWQbHrwD6CV6qqSyazKjtlN8ClZuc=.ed25519": 9,
				"@eANNuLfzX/9rlGODXHYV8WJb+zw2h+d7YsT4vpYPvD0=.ed25519": 4404,
				"@fBS90Djngwl/SlCh/20G7piSC064Qz2hBBxbfnbyM+Y=.ed25519": 2244,
				"@j4Me5k19XjCyGKoCPOhohjpLq7TxmiWtrG/YhFK7AYI=.ed25519": 201,
				"@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519": 12444,
				"@qv10rF4IsmxRZb7g5ekJ33EakYBpdrmV/vtP1ij5BS4=.ed25519": 981,
				"@t5iZ/IQAYnYsGzQdpTKvbLpZnlmUML408BrjsvlwRew=.ed25519": 334,
				"@wKaaftUxisV3GA3zuk2ena5krzJgB/6HwvgOKKIR/jQ=.ed25519": 8,
				"@xxZeuBUKIXYZZ7y+CIHUuy0NOFXz2MhUBkHemr86S3M=.ed25519": 526,
				"@1nXNP4vDRn2Y7LsPA9/VTRnFOOKjsjf2ioeA2wW50KQ=.ed25519": 4,
				"@4IJsfS0cntIiHt4DkuPtp/PUNlR+FLTD0Gr2HVhuQ+E=.ed25519": 132,
				"@5kW8r/g1mMO5qD95a7wSFI23zLAeZeVCBpAHZptMmqg=.ed25519": 25,
				"@+D0ku/LReK6kqd3PSrcVCfbLYbDtTmS4Bd21rqhpYNA=.ed25519": 4163,
			},
		},

		{
			Name:   "large",
			Length: 125000,
			HeadCount: tFeedSet{
				"@Aj30ftcGjrxufcB7xfGxlyTb+SswXacEsPjC8I8I3XY=.ed25519": 240,
				"@CxnSXWYjPT160y7QbmTFtWaWT09080azgErYPt1ZeZc=.ed25519": 4853,
				"@C/zyw6xOd/QIcTLuZKZcrMoMmTwMfQs0u0rWEhNksgw=.ed25519": 30,
				"@DJVbfRTEhC3cH1zU5/BAx6MgKIX1fkkytaRziA10suw=.ed25519": 306,
				"@D26sJ/Seyc4WBcpZDi4PYcqEg+2nUb7WoTQg9NknDyg=.ed25519": 1875,
				"@EMovhfIrFk4NihAKnRNhrfRaqIhBv1Wj8pTxJNgvCCY=.ed25519": 8111,
				"@FWHrnoH/YwtiVSng8MiWrb6ekZDX9R+i3XlXLlmtaiE=.ed25519": 404,
				"@HDFD0DqC4yxMItsq9jrbj3ZGgTzThbjCuApdFpWw6v8=.ed25519": 11,
				"@HEqy940T6uB+T+d9Jaa58aNfRzLx9eRWqkZljBmnkmk=.ed25519": 8347,
				"@HP+nJCFW8UgDVXY93NhYMfGM/DotOblJq71nm4S4rW4=.ed25519": 1,
				"@Hn9ye+0RgZDXvp+Km4EC36pN1/xB5ultjTxC0RZW2BM=.ed25519": 1552,
				"@IgYpd+tCtXnlE2tYX/8rR2AGt+P8svC98WH3MdYAa8Y=.ed25519": 3964,
				"@LtQ3tOuLoeQFi5s/ic7U6wDBxWS3t2yxauc4/AwqfWc=.ed25519": 13,
				"@MRiJ+CvDnD9ZjqunY1oy6tsk0IdbMDC4Q3tTC8riS3s=.ed25519": 1203,
				"@NXNrTfdIIQpdG9CAXPKaEoB7c4IFgaGzpNy9le+1VZw=.ed25519": 60,
				"@NeB4q4Hy9IiMxs5L08oevEhivxW+/aDu/s/0SkNayi0=.ed25519": 8588,
				"@OF30xmIi8yCA6nLuu8UtCxy+LAzRJ3yzH/s6Ia6ejGs=.ed25519": 138,
				"@Oqa5JW8rwWIVLBu38KkIb1IYz6Ax0yHuwRvLEGR1mkY=.ed25519": 460,
				"@QRfMpQtngucdDQUx0yhanKtbM3UGjj9bsHGF65Iaqfo=.ed25519": 16,
				"@Q6jeOaoJOdFq8/3oFaOYC6bKhaVCD3IIdpjG+7Fab2M=.ed25519": 5034,
				"@SWS8CabsODBbrydRxAD3Q95ABxrneXdj6/eLm/Xne3Y=.ed25519": 496,
				"@U5GvOKP/YUza9k53DSXxT0mk3PIrnyAmessvNfZl5E0=.ed25519": 4372,
				"@XbgAjnHAIESiABEGuJOAojN4zkZcPaF34/YRbpOHyiI=.ed25519": 831,
				"@ZGZtWXjv6HgT7ZZlfKl4d+fhKglLRHkAAqjr5fvqDgM=.ed25519": 135,
				"@arDWwU9A6/JxjaCWQbHrwD6CV6qqSyazKjtlN8ClZuc=.ed25519": 9,
				"@bZB4VjCoBpQjbMDJba1lOyCzp3X7dWMG8iY0JvbDQHg=.ed25519": 15,
				"@c79QS2Ige79DqHr9CwSOGwhYL74qyhGdZCFyRSPHpks=.ed25519": 50,
				"@dFDZE1NsZmS2mwWNho80m1DZWZP9iGD/fWSXwPZr0vg=.ed25519": 451,
				"@eANNuLfzX/9rlGODXHYV8WJb+zw2h+d7YsT4vpYPvD0=.ed25519": 4404,
				"@fBS90Djngwl/SlCh/20G7piSC064Qz2hBBxbfnbyM+Y=.ed25519": 2244,
				"@f/6sQ6d2CMxRUhLpspgGIulDxDCwYD7DzFzPNr7u5AU=.ed25519": 14123,
				"@gaQw6z30GpfsW9k8V5ED4pHrg8zmrqku24zTSAINhRg=.ed25519": 4298,
				"@j4Me5k19XjCyGKoCPOhohjpLq7TxmiWtrG/YhFK7AYI=.ed25519": 201,
				"@ppdSxn1pSozJIqtDE4pYgwaQGmswCT9y15VJJcXRntI=.ed25519": 3932,
				"@p13zSAiOpguI9nsawkGijsnMfWmFd5rlUNpzekEE+vI=.ed25519": 12444,
				"@qv10rF4IsmxRZb7g5ekJ33EakYBpdrmV/vtP1ij5BS4=.ed25519": 981,
				"@t5iZ/IQAYnYsGzQdpTKvbLpZnlmUML408BrjsvlwRew=.ed25519": 896,
				"@ufDsUB8NZZf9HheecsAXYTSFzuByRYbH4qSmia5ASpo=.ed25519": 4,
				"@uikkwUQU4dcd/ZrHU7JstnkTgncxQB2A8PDLHV9wDAs=.ed25519": 4576,
				"@wKaaftUxisV3GA3zuk2ena5krzJgB/6HwvgOKKIR/jQ=.ed25519": 8,
				"@w7wcvOudsg5rLtM1J9vHcCT/k5PW+6ahsnY34fGEa4E=.ed25519": 302,
				"@xxZeuBUKIXYZZ7y+CIHUuy0NOFXz2MhUBkHemr86S3M=.ed25519": 526,
				"@yARQ14OOZXgI8OU8UsgQhZSSmzPbuwCgC5gXxmRAhkE=.ed25519": 84,
				"@yaezRMnzfpQVsnqZ3L3IPa7s5dWPY9B0ZiZVCEujnKY=.ed25519": 38,
				"@ya/sq19NPxRza5xtoqi9BilwLZ7HgQjG3QpcTRnGgWs=.ed25519": 2211,
				"@z8aJVHJTc6MM8FwaNE2GIS3AYYt2HYFNWQUO8/iydNw=.ed25519": 3850,
				"@1DfC2qFuXuli/HOg3kJbKwxiOpc3jXLdJD3TnhtzWNs=.ed25519": 236,
				"@1nXNP4vDRn2Y7LsPA9/VTRnFOOKjsjf2ioeA2wW50KQ=.ed25519": 4,
				"@3ZeNUiYQZisGC6PLf3R+u2s5avtxLsXC66xuK41e6Zk=.ed25519": 4360,
				"@3r4+IyB5NVl2in6QOZHIu9oSrZud+NuVgl2GX3x2WG8=.ed25519": 3464,
				"@4IJsfS0cntIiHt4DkuPtp/PUNlR+FLTD0Gr2HVhuQ+E=.ed25519": 132,
				"@5fYRrgyJON0r0R9SPrK0oxR1XKhqNXqiN0FBX+MgfH4=.ed25519": 1310,
				"@5kW8r/g1mMO5qD95a7wSFI23zLAeZeVCBpAHZptMmqg=.ed25519": 25,
				"@9nTgtYmvW4HID6ayt6Icwc8WZxdifx5SlSKKIX/X/1g=.ed25519": 1552,
				"@+D0ku/LReK6kqd3PSrcVCfbLYbDtTmS4Bd21rqhpYNA=.ed25519": 6699,
				"@/YL+qpivBMZShNAmbNidr27Z96htm5QLQ1JDPtEMIQM=.ed25519": 475,
			},
		},
	}

	for _, tc := range cases {
		tr := repo.New(filepath.Join("testdata", tc.Name))

		testLog, err := repo.OpenLog(tr)
		r.NoError(err, "case %s failed to open", tc.Name)

		sv, err := testLog.Seq().Value()
		r.NoError(err)
		if !a.EqualValues(tc.Length, sv.(margaret.Seq).Seq()+1, "case %s has wrong number of messages", tc.Name) {
			continue
		}

		start := time.Now()
		roarmlog, serve, err := OpenUserFeeds(tr)
		r.NoError(err)

		err = serve(context.Background(), testLog, false)
		r.NoError(err)
		t.Log("indexing  mlog", tc.Name, "took", time.Since(start))

		compare := func(ml multilog.MultiLog) {
			addrs, err := ml.List()
			r.NoError(err)
			a.Equal(len(tc.HeadCount), len(addrs))

			for i, addr := range addrs {
				var sr ssb.StorageRef
				err := sr.Unmarshal([]byte(addr))
				r.NoError(err, "ref %d invalid", i)

				seq, has := tc.HeadCount[sr.Ref()]

				sublog, err := ml.Get(addr)
				r.NoError(err)

				sv, err := sublog.Seq().Value()
				r.NoError(err)
				sublogSeq := sv.(margaret.Seq).Seq()

				if a.True(has, "ref %s not in set (has:%d)", sr.Ref(), sublogSeq) {

					a.EqualValues(seq, sublogSeq, "%s: sublog %s has wrong number of messages", tc.Name, sr.Ref())
				}
			}
			r.NoError(ml.Close())
		}

		compare(roarmlog)

		start = time.Now()
		badgermlog, serve, err := repo.OpenBadgerMultiLog(tr, "testbadger_"+tc.Name, UserFeedsUpdate)
		r.NoError(err)

		err = serve(context.Background(), testLog, false)
		r.NoError(err)
		t.Log("indexing  roar", tc.Name, "took", time.Since(start))

		compare(badgermlog)

	}
}

func benchSequential(i int) func(b *testing.B) {
	return func(b *testing.B) {
		r := require.New(b)
		ctx := context.TODO()

		testRepo := repo.New(filepath.Join("testdata", "medium"))
		testLog, err := repo.OpenLog(testRepo)
		r.NoError(err)

		src, err := testLog.Query(margaret.Limit(i))

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for {
				v, err := src.Next(ctx)
				if luigi.IsEOS(err) {
					break
				} else {
					r.NoError(err)
				}
				_, ok := v.(refs.Message)
				r.True(ok)
				// r.NotNil(msg.Key())
				// r.NotNil(msg.Author())
			}

		}
	}
}

func BenchmarkSequential(b *testing.B) {
	b.Run("100", benchSequential(100))
	b.Run("5k", benchSequential(5000))
	b.Run("20k", benchSequential(20000))
}

func benchRandom(i int) func(b *testing.B) {
	return func(b *testing.B) {
		r := require.New(b)

		testRepo := repo.New(filepath.Join("testdata", "medium"))
		testLog, err := repo.OpenLog(testRepo)
		r.NoError(err)

		sv, err := testLog.Seq().Value()
		r.NoError(err)
		r.EqualValues(50000, sv.(margaret.Seq).Seq()+1)

		var seqs []margaret.Seq
		for j := i; j > 0; j-- {
			seqs = append(seqs, margaret.BaseSeq(rand.Int63n(50000)))

		}

		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			for _, seq := range seqs {
				v, err := testLog.Get(seq)
				r.NoError(err)
				_, ok := v.(refs.Message)
				r.True(ok)
				// r.NotNil(msg.Key())
				// r.NotNil(msg.Author())
			}

		}
	}
}

func BenchmarkRandom(b *testing.B) {
	b.Run("100", benchRandom(100))
	b.Run("5k", benchRandom(5000))
	b.Run("20k", benchRandom(20000))
}
