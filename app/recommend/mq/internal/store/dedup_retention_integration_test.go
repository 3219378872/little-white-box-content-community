//go:build integration

package store

import (
	"context"
	"fmt"
	"testing"

	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
)

func TestRedisBehaviorReceiptsRetainNinetyDaysWhileFeaturesKeepThirty(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	for _, day := range []int{31, 89} {
		t.Run(fmt.Sprint(day), func(t *testing.T) {
			version := fmt.Sprintf("dedup-day-%d", day)
			prefix := "feature:" + version
			store := NewRedisBehaviorStoreWithRetention(env.Redis, version, "recommend", 2592000, 7776000, enabledPreferences())
			behavior := behaviorEvent(int64(day), 42, "exposure", 99, "post")
			behavior.RequestID = "request"
			behavior.Position = new(int32(1))
			require.NoError(t, store.Record(ctx, behavior))
			keys := []string{prefix + ":dedup:" + behavior.EventIDString(), prefix + ":u:42:exposure:dedup:request:99"}
			for _, key := range keys {
				ttl, err := env.Redis.TtlCtx(ctx, key)
				require.NoError(t, err)
				require.InDelta(t, 7776000, ttl, 5)
				require.NoError(t, env.Redis.ExpireCtx(ctx, key, (90-day)*24*3600))
			}
			// Simulate elapsed receipt age while activity has refreshed the feature TTL.
			require.NoError(t, env.Redis.ExpireCtx(ctx, prefix+":u:42:recent", 2592000))
			restart := NewRedisBehaviorStore(env.Redis, version, "recommend", 2592000, enabledPreferences())
			require.NoError(t, restart.Record(ctx, behavior))
			behavior.EventID++
			behavior.ClientEventID += "-another"
			require.NoError(t, restart.Record(ctx, behavior))
			ranked, err := env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, "recommend:"+version+":recall:post:hot:home", 0, -1)
			require.NoError(t, err)
			require.Len(t, ranked, 1)
			require.Equal(t, float64(1), ranked[0].Score)
			ttl, err := env.Redis.TtlCtx(ctx, prefix+":u:42:recent")
			require.NoError(t, err)
			require.InDelta(t, 2592000, ttl, 5)
		})
	}
}
func TestRedisDedupMigrationIsIdempotentAndPreservesReceiptAge(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	store := NewRedisBehaviorStore(env.Redis, "migration", "recommend", 2592000, enabledPreferences())
	keys := []string{"feature:migration:dedup:1", "feature:migration:u:42:exposure:dedup:r:9"}
	for _, key := range keys {
		require.NoError(t, env.Redis.SetexCtx(ctx, key, "1", 10*24*3600))
	}
	n, err := store.UpgradeDedupRetention(ctx, 30*24*3600)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	for _, key := range keys {
		ttl, err := env.Redis.TtlCtx(ctx, key)
		require.NoError(t, err)
		require.InDelta(t, 70*24*3600, ttl, 5)
	}
	n, err = store.UpgradeDedupRetention(ctx, 30*24*3600)
	require.NoError(t, err)
	require.Zero(t, n)
	for _, key := range keys {
		ttl, err := env.Redis.TtlCtx(ctx, key)
		require.NoError(t, err)
		require.InDelta(t, 70*24*3600, ttl, 5)
	}
}
