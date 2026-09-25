//go:build integration

package store

import (
	"context"
	"esx/pkg/event"
	"esx/pkg/testutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDurableOptOutSurvivesExpiredMarkerAndStopsPostFanout(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	preference := enabledPreferences()
	behavior := NewRedisBehaviorStore(env.Redis, "v2-privacy", "recommend", 3600, preference)
	candidates := NewRedisCandidateStore(env.Redis, "v2-privacy", "recommend", 3600, preference)
	require.NoError(t, behavior.Record(ctx, behaviorEvent(1, 42, "follow", 7, "user")))
	require.NoError(t, behavior.Record(ctx, behaviorEvent(2, 42, "like", 91, "post")))
	preference.enabled = false
	require.NoError(t, env.Redis.SetexCtx(ctx, "personalization:optout:42", "1", 7*24*3600))
	require.NoError(t, env.Redis.ExpireCtx(ctx, "personalization:optout:42", 0))
	marker, err := env.Redis.GetCtx(ctx, "personalization:optout:42")
	require.NoError(t, err)
	require.Empty(t, marker)
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{EventID: 3, EventTime: 1000, Type: event.PostEventCreated, PostID: 92, AuthorID: 7}))
	posts, err := env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, "recommend:v2-privacy:recall:post:follow:u:42:home", 0, -1)
	require.NoError(t, err)
	require.Empty(t, posts, "new posts must not recreate a disabled profile when marker expires")
	require.NoError(t, behavior.Record(ctx, behaviorEvent(4, 42, "like", 93, "post")))
	members, err := env.Redis.SmembersCtx(ctx, "feature:v2-privacy:u:42:positive")
	require.NoError(t, err)
	require.Empty(t, members)
	followers, err := env.Redis.SmembersCtx(ctx, "recommend:v2-privacy:follow:author:7:followers")
	require.NoError(t, err)
	require.NotContains(t, followers, "u:42")
}

func TestScheduledCleanupFindsOptOutAfterMarkerWriteWasLost(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	preference := enabledPreferences()
	behavior := NewRedisBehaviorStore(env.Redis, "v2-cleanup", "recommend", 3600, preference)
	candidates := NewRedisCandidateStore(env.Redis, "v2-cleanup", "recommend", 3600, preference)
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{EventID: 1, EventTime: 1000, Type: event.PostEventCreated, PostID: 91, AuthorID: 7}))
	require.NoError(t, behavior.Record(ctx, behaviorEvent(2, 42, "follow", 7, "user")))
	preference.enabled = false // DB committed, but no opt-out marker made it into Redis.
	count, err := behavior.PurgeOptedOutFeatures(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	for _, pattern := range []string{"feature:v2-cleanup:u:42:*", "recommend:v2-cleanup:recall:post:follow:u:42:*"} {
		keys, err := env.Redis.KeysCtx(ctx, pattern)
		require.NoError(t, err)
		require.Empty(t, keys)
	}
	followers, err := env.Redis.SmembersCtx(ctx, "recommend:v2-cleanup:follow:author:7:followers")
	require.NoError(t, err)
	require.NotContains(t, followers, "u:42")
}
