package snapshot

import (
	"testing"

	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

func TestFreezeIgnoresInvisibleAndWidthVariants(t *testing.T) {
	plain := event.ReviewSnapshot{
		Texts:  map[string]string{"title": "Best Price 100%", "body": "buy now"},
		Market: "US", Language: "en", SubmitterID: 7,
		Media: []event.ReviewMedia{{MediaID: 2, SHA256: "BB"}, {MediaID: 1, SHA256: "aa"}},
	}
	obfuscated := event.ReviewSnapshot{
		Texts:  map[string]string{"title": "Ｂｅｓｔ\u200b Price  １００％", "body": " buy\u00adnow "},
		Market: "us", Language: "EN", SubmitterID: 7,
		Media: []event.ReviewMedia{{MediaID: 1, SHA256: "AA"}, {MediaID: 2, SHA256: "bb"}},
	}
	obfuscated.Texts["body"] = " buy now\xef\xbb\xbf"
	a, err := Freeze(event.ReviewBizAdCreative, plain)
	require.NoError(t, err)
	b, err := Freeze(event.ReviewBizAdCreative, obfuscated)
	require.NoError(t, err)
	require.Equal(t, a.Hash, b.Hash)
	require.Equal(t, "aa", a.Snapshot.Media[0].SHA256)
}

func TestFreezeSeparatesBizTypesAndContent(t *testing.T) {
	snap := event.ReviewSnapshot{Texts: map[string]string{"title": "x"}, Market: "US", SubmitterID: 1}
	a, err := Freeze(event.ReviewBizAdCreative, snap)
	require.NoError(t, err)
	b, err := Freeze(event.ReviewBizAdvertiserQualification, snap)
	require.NoError(t, err)
	require.NotEqual(t, a.Hash, b.Hash)
	snap.Texts = map[string]string{"title": "y"}
	c, err := Freeze(event.ReviewBizAdCreative, snap)
	require.NoError(t, err)
	require.NotEqual(t, a.Hash, c.Hash)
}

func TestNormalizeDoesNotMutateInput(t *testing.T) {
	in := event.ReviewSnapshot{Texts: map[string]string{"title": "Ａ"}, Market: "us", SubmitterID: 1}
	_ = Normalize(in)
	require.Equal(t, "Ａ", in.Texts["title"])
	require.Equal(t, "us", in.Market)
}
