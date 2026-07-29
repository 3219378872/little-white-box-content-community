//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestRedisCandidatePipelineProducesOnlineRecallKeys(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	candidates := NewRedisCandidateStore(env.Redis, "v2", "recommend", 3600)
	behaviors := NewRedisBehaviorStore(env.Redis, "v2", "recommend", 3600)

	for _, postID := range []int64{101, 102} {
		require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
			EventID: postID, EventTime: 1_000 + postID, Type: event.PostEventCreated,
			PostID: postID, AuthorID: 7, Title: "post", Tags: []string{"go"},
		}))
	}
	require.NoError(t, behaviors.Record(ctx, behaviorEvent(1, 42, "follow", 7, "user")))

	followKey := "recommend:v2:recall:post:follow:u:42:home"
	followPosts, err := env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, followKey, 0, -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"101", "102"}, pairKeys(followPosts))

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 103, EventTime: 1_103, Type: event.PostEventCreated,
		PostID: 103, AuthorID: 7, Title: "new post", Tags: []string{"go"},
	}))
	followPosts, err = env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, followKey, 0, -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"101", "102", "103"}, pairKeys(followPosts))

	require.NoError(t, behaviors.Record(ctx, behaviorEvent(2, 42, "like", 101, "post")))
	require.NoError(t, behaviors.Record(ctx, behaviorEvent(3, 42, "like", 102, "post")))
	require.NoError(t, behaviors.Record(ctx, behaviorEvent(4, 43, "like", 101, "post")))
	itemCF, err := env.Redis.ZrevrangeWithScoresByFloatCtx(
		ctx, "recommend:v2:recall:post:itemcf:u:43:home", 0, -1,
	)
	require.NoError(t, err)
	assert.Contains(t, pairKeys(itemCF), "102")

	features, err := env.Redis.HgetallCtx(ctx, "feature:v2:post:101")
	require.NoError(t, err)
	assert.Equal(t, "active", features["status"])
	assert.Equal(t, "7", features["author_id"])
	assert.Equal(t, "go", features["category"])
	assert.NotEmpty(t, features["popularity"])

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 104, EventTime: 2_000, Type: event.PostEventDeleted, PostID: 103, AuthorID: 7,
	}))
	followPosts, err = env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, followKey, 0, -1)
	require.NoError(t, err)
	assert.NotContains(t, pairKeys(followPosts), "103")
	features, err = env.Redis.HgetallCtx(ctx, "feature:v2:post:103")
	require.NoError(t, err)
	assert.Equal(t, "deleted", features["status"])
}

func TestRedisDeadLettersAreBounded(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	recorder := NewRedisDeadLetterRecorder(env.Redis, "recommend", "v2", 3600, 2)
	for id := 1; id <= 3; id++ {
		require.NoError(t, recorder.RecordDeadLetter(
			context.Background(), strconv.Itoa(id), []byte(`bad`), errors.New("invalid"),
		))
	}
	length, err := env.Redis.LlenCtx(context.Background(), "recommend:v2:dead-letters")
	require.NoError(t, err)
	assert.Equal(t, 2, length)
}

func TestRedisBehaviorStoreKeepsEventTimeOrderAndLatestFollowState(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	store := NewRedisBehaviorStore(env.Redis, "v2-ordering", "recommend", 3600)
	candidates := NewRedisCandidateStore(env.Redis, "v2-ordering", "recommend", 3600)

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 201, EventTime: 1_000, Type: event.PostEventCreated,
		PostID: 201, AuthorID: 7, Title: "post",
	}))
	newerUnfollow := behaviorEvent(10, 42, event.BehaviorActionUnfollow, 7, "user")
	newerUnfollow.EventTime = 3_000
	olderFollow := behaviorEvent(11, 42, event.BehaviorActionFollow, 7, "user")
	olderFollow.EventTime = 2_000
	require.NoError(t, store.Record(ctx, newerUnfollow))
	require.NoError(t, store.Record(ctx, olderFollow))

	followers, err := env.Redis.SmembersCtx(ctx, "recommend:v2-ordering:follow:author:7:followers")
	require.NoError(t, err)
	assert.NotContains(t, followers, "u:42")
	followPosts, err := env.Redis.ZrevrangeWithScoresByFloatCtx(
		ctx, "recommend:v2-ordering:recall:post:follow:u:42:home", 0, -1,
	)
	require.NoError(t, err)
	assert.Empty(t, followPosts)

	for _, item := range []struct {
		id        int64
		eventTime int64
	}{{20, 6_000}, {21, 4_000}, {22, 5_000}} {
		behavior := behaviorEvent(item.id, 42, event.BehaviorActionClick, item.id, "post")
		behavior.EventTime = item.eventTime
		require.NoError(t, store.Record(ctx, behavior))
	}
	recent, err := env.Redis.LrangeCtx(ctx, "feature:v2-ordering:u:42:recent", 0, 49)
	require.NoError(t, err)
	recentTimes := make([]int64, 0, len(recent))
	for _, raw := range recent {
		var behavior event.BehaviorEvent
		require.NoError(t, json.Unmarshal([]byte(raw), &behavior))
		recentTimes = append(recentTimes, behavior.EventTime)
	}
	assert.Equal(t, []int64{6_000, 5_000, 4_000, 3_000, 2_000}, recentTimes)
}

func behaviorEvent(id, userID int64, action string, targetID int64, targetType string) event.BehaviorEvent {
	return event.BehaviorEvent{
		EventID: id, ClientEventID: "client-" + strconv.FormatInt(id, 10),
		SchemaVersion: event.BehaviorSchemaVersion, EventTime: 1_000 + id, ReceivedAt: 2_000 + id,
		UserID: userID, Action: action, TargetID: targetID, TargetType: targetType,
		Scene: "home", Producer: "test",
	}
}

func pairKeys(pairs []redis.FloatPair) []string {
	keys := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		keys = append(keys, pair.Key)
	}
	return keys
}
