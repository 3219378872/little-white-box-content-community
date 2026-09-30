//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"esx/app/recommend/feedback"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
)

func TestExplicitExclusionsSurviveOptOutAndAcceptNewHides(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	preference := enabledPreferences()
	store := NewRedisBehaviorStore(env.Redis, "v2-required", "recommend", 3600, preference)
	reader := feedback.NewHiddenPostReader(env.Redis, "v2-required")
	require.NoError(t, store.Record(ctx, behaviorEvent(1, 42, "hide", 91, "post")))
	require.NoError(t, store.Record(ctx, behaviorEvent(2, 42, "like", 92, "post")))
	preference.enabled = false
	_, err := store.PurgeOptedOutFeatures(ctx)
	require.NoError(t, err)
	hidden, err := reader.HiddenPosts(ctx, 42)
	require.NoError(t, err)
	require.Contains(t, hidden, int64(91))
	require.NoError(t, store.Record(ctx, behaviorEvent(3, 42, "dislike", 93, "post")))
	hidden, err = reader.HiddenPosts(ctx, 42)
	require.NoError(t, err)
	require.Contains(t, hidden, int64(91))
	require.Contains(t, hidden, int64(93))
	for _, suffix := range []string{"recent", "positive", "scene", "state"} {
		exists, err := env.Redis.ExistsCtx(ctx, "feature:v2-required:u:42:"+suffix)
		require.NoError(t, err)
		require.False(t, exists)
	}
	// Unknown personalization preference still permits a required direct hide,
	// while the narrow script creates no optional profile.
	preference.err = errors.New("preference unavailable")
	require.NoError(t, store.Record(ctx, behaviorEvent(4, 42, "hide", 94, "post")))
	hidden, err = reader.HiddenPosts(ctx, 42)
	require.NoError(t, err)
	require.Contains(t, hidden, int64(94))
}
