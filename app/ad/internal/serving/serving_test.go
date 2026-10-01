package serving

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIdentityNeverUsesAnonymousID(t *testing.T) {
	id, err := Identity(7, "session")
	require.NoError(t, err)
	require.Equal(t, "u:7", id)
	a, err := Identity(0, "session-a")
	require.NoError(t, err)
	b, err := Identity(0, "session-b")
	require.NoError(t, err)
	require.NotEqual(t, a, b)
	require.NotContains(t, a, "session-a")
	_, err = Identity(0, "")
	require.ErrorIs(t, err, ErrNoIdentity)
}

func TestFreqKeyUsesUTCDay(t *testing.T) {
	shanghai := time.FixedZone("CST", 8*3600)
	late := time.Date(2026, 10, 2, 7, 0, 0, 0, shanghai) // 2026-10-01 23:00 UTC
	require.Equal(t, "ads:v1:freq:u:1:20261001", FreqKey("u:1", late))
	require.NotEqual(t, RequestKey("r", "c1"), RequestKey("r", "c2"))
}

func TestChooseRespectsCapAdvertiserAndPositions(t *testing.T) {
	candidates := []Candidate{
		{Entry: Entry{AdID: 1, AdvertiserID: 10}, TodayCount: 3},
		{Entry: Entry{AdID: 2, AdvertiserID: 20}, TodayCount: 1},
		{Entry: Entry{AdID: 3, AdvertiserID: 20}, TodayCount: 0},
		{Entry: Entry{AdID: 4, AdvertiserID: 30}, TodayCount: 2},
	}
	got := Choose("req", 20, candidates)
	require.Len(t, got, 2)
	require.Equal(t, int64(3), got[0].Entry.AdID) // 当日最少
	require.Equal(t, 4, got[0].AfterIndex)
	require.Equal(t, "s1", got[0].SlotID)
	require.Equal(t, int64(4), got[1].Entry.AdID) // 广告主 20 已占用，广告 1 已达上限
	require.Equal(t, 12, got[1].AfterIndex)

	require.Len(t, Choose("req", 10, candidates), 1)
	require.Empty(t, Choose("req", 3, candidates))
	require.Equal(t, Choose("same", 20, candidates), Choose("same", 20, candidates))
}

func TestInWindow(t *testing.T) {
	require.True(t, Entry{}.InWindow(5))
	require.False(t, Entry{StartMs: 10}.InWindow(5))
	require.False(t, Entry{EndMs: 5}.InWindow(5))
	require.True(t, Entry{StartMs: 1, EndMs: 10}.InWindow(5))
}
