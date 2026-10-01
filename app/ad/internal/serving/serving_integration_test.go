//go:build integration

package serving

import (
	"context"
	"testing"
	"time"

	redis "esx/pkg/redisstore"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
)

func TestRedisFrequencyHideAndIndex(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	defer env.Close()
	kv := redis.MustNewRedis(redis.RedisConf{Host: env.Addr})
	ctx := context.Background()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	// ADS-024：每个 UTC 自然日最多 3 次；不同身份互不影响；次日重置。
	for i := range 3 {
		granted, err := Reserve(ctx, kv, "u:1", []int64{10, 11}, now)
		require.NoError(t, err, i)
		require.Equal(t, []int64{10, 11}, granted)
	}
	granted, err := Reserve(ctx, kv, "u:1", []int64{10, 12}, now)
	require.NoError(t, err)
	require.Equal(t, []int64{12}, granted)
	other, err := Reserve(ctx, kv, "s:abc", []int64{10}, now)
	require.NoError(t, err)
	require.Equal(t, []int64{10}, other)
	nextDay, err := Reserve(ctx, kv, "u:1", []int64{10}, now.Add(24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, []int64{10}, nextDay)
	counts, err := Counts(ctx, kv, "u:1", now)
	require.NoError(t, err)
	require.Equal(t, int64(3), counts[10])
	ttl, err := kv.TtlCtx(ctx, FreqKey("u:1", now))
	require.NoError(t, err)
	require.Positive(t, ttl)

	// ADS-026：隐藏 30 天；匿名会话键短期失效。
	require.NoError(t, Hide(ctx, kv, "u:1", 10, now))
	hidden, err := Hidden(ctx, kv, "u:1", now)
	require.NoError(t, err)
	require.True(t, hidden[10])
	hidden, err = Hidden(ctx, kv, "u:1", now.Add(31*24*time.Hour))
	require.NoError(t, err)
	require.False(t, hidden[10])
	require.NoError(t, Hide(ctx, kv, "s:abc", 10, now))
	sessionTTL, err := kv.TtlCtx(ctx, HiddenKey("s:abc"))
	require.NoError(t, err)
	require.LessOrEqual(t, sessionTTL, int(SessionHideTTL.Seconds()))

	index := Index{KV: kv}
	require.NoError(t, index.Upsert(ctx, "US", Entry{AdID: 1, Revision: 2, AdvertiserID: 3}))
	require.NoError(t, index.Upsert(ctx, "US", Entry{AdID: 4, Revision: 1, AdvertiserID: 5}))
	entries, err := index.Load(ctx, "US")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.NoError(t, index.Remove(ctx, "US", 1))
	entries, err = index.Load(ctx, "US")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, int64(4), entries[0].AdID)
}
