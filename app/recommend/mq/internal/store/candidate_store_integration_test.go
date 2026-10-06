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

	redis "esx/pkg/redisstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisCandidatePipelineProducesOnlineRecallKeys(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	candidates := NewRedisCandidateStore(env.Redis, "v2", "recommend", 3600, enabledPreferences())
	behaviors := NewRedisBehaviorStore(env.Redis, "v2", "recommend", 3600, enabledPreferences())

	for _, postID := range []int64{101, 102} {
		require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
			EventID: postID, EventTime: 1_000 + postID, Type: event.PostEventCreated, Status: 1,
			PostID: postID, AuthorID: 7, Title: "post", Tags: []string{"go"},
		}))
	}
	require.NoError(t, behaviors.Record(ctx, behaviorEvent(1, 42, "follow", 7, "user")))

	followKey := "recommend:v2:recall:post:follow:u:42:home"
	followPosts, err := env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, followKey, 0, -1)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"101", "102"}, pairKeys(followPosts))

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 103, EventTime: 1_103, Type: event.PostEventCreated, Status: 1,
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

func TestRedisCandidateStoreIgnoresStaleRevision(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	candidates := NewRedisCandidateStore(env.Redis, "v2-rev", "recommend", 3600, enabledPreferences())

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 302, EventTime: 2_000, Type: event.PostEventUpdated, Status: 1,
		PostID: 300, AuthorID: 7, Title: "C", Tags: []string{"c-tag"}, Revision: 3,
	}))
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 301, EventTime: 1_000, Type: event.PostEventUpdated, Status: 1,
		PostID: 300, AuthorID: 7, Title: "B", Tags: []string{"b-tag"}, Revision: 2,
	}))

	features, err := env.Redis.HgetallCtx(ctx, "feature:v2-rev:post:300")
	require.NoError(t, err)
	assert.Equal(t, "c-tag", features["category"])
	assert.Equal(t, "3", features["revision"])
}

func TestRedisCandidateStoreCountPatchesKeepCandidateState(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	candidates := NewRedisCandidateStore(env.Redis, "v2-count", "recommend", 3600, enabledPreferences())
	hotKey := "recommend:v2-count:recall:post:hot:home"

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 401, EventTime: 1_000, Type: event.PostEventUpdated, Status: 1,
		PostID: 400, AuthorID: 7, Tags: []string{"go"}, Revision: 3,
	}))
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 402, EventTime: 9_000, Type: event.PostEventCounted,
		PostID: 400, LikeCount: 5, CommentCount: 1, StatsSeq: 9_000,
	}))
	features, err := env.Redis.HgetallCtx(ctx, "feature:v2-count:post:400")
	require.NoError(t, err)
	assert.Equal(t, "7", features["author_id"])
	assert.Equal(t, "go", features["category"])
	assert.Equal(t, "3", features["revision"])
	score, err := env.Redis.ZscoreCtx(ctx, hotKey, "400")
	require.NoError(t, err)
	assert.Equal(t, int64(1_000), score, "count patches must not refresh recall time")
	orphan, err := env.Redis.ExistsCtx(ctx, "recommend:v2-count:author:0:posts")
	require.NoError(t, err)
	assert.False(t, orphan)

	// The revision guard survives the count patch, so a late snapshot is dropped.
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 403, EventTime: 500, Type: event.PostEventUpdated, Status: 1,
		PostID: 400, AuthorID: 7, Tags: []string{"stale"}, Revision: 2,
	}))
	features, err = env.Redis.HgetallCtx(ctx, "feature:v2-count:post:400")
	require.NoError(t, err)
	assert.Equal(t, "go", features["category"])

	// A count patch after deletion must not revive the candidate.
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 404, EventTime: 2_000, Type: event.PostEventDeleted, PostID: 400, AuthorID: 7, Revision: 4,
	}))
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 405, EventTime: 3_000, Type: event.PostEventCounted, PostID: 400, LikeCount: 6,
	}))
	features, err = env.Redis.HgetallCtx(ctx, "feature:v2-count:post:400")
	require.NoError(t, err)
	assert.Equal(t, "deleted", features["status"])
	hot, err := env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, hotKey, 0, -1)
	require.NoError(t, err)
	assert.NotContains(t, pairKeys(hot), "400")
}

func TestRedisCandidateStoreUnpublishRemovesRecallAndRepublishRestores(t *testing.T) {
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)
	ctx := context.Background()
	candidates := NewRedisCandidateStore(env.Redis, "v2-draft", "recommend", 3600, enabledPreferences())
	behaviors := NewRedisBehaviorStore(env.Redis, "v2-draft", "recommend", 3600, enabledPreferences())
	keys := []string{
		"recommend:v2-draft:recall:post:hot:home",
		"recommend:v2-draft:recall:post:explore:home",
		"recommend:v2-draft:author:7:posts",
		"recommend:v2-draft:recall:post:follow:u:42:home",
	}
	recalled := func() []bool {
		present := make([]bool, len(keys))
		for index, key := range keys {
			members, err := env.Redis.ZrevrangeWithScoresByFloatCtx(ctx, key, 0, -1)
			require.NoError(t, err)
			for _, member := range pairKeys(members) {
				present[index] = present[index] || member == "500"
			}
		}
		return present
	}

	// A draft never enters recall.
	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 501, EventTime: 1_000, Type: event.PostEventCreated, Status: 0,
		PostID: 500, AuthorID: 7, Revision: 1,
	}))
	require.NoError(t, behaviors.Record(ctx, behaviorEvent(510, 42, "follow", 7, "user")))
	assert.Equal(t, []bool{false, false, false, false}, recalled())
	features, err := env.Redis.HgetallCtx(ctx, "feature:v2-draft:post:500")
	require.NoError(t, err)
	assert.Equal(t, "unpublished", features["status"])

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 502, EventTime: 2_000, Type: event.PostEventUpdated, Status: 1,
		PostID: 500, AuthorID: 7, Revision: 2,
	}))
	assert.Equal(t, []bool{true, true, true, true}, recalled())

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 503, EventTime: 3_000, Type: event.PostEventUpdated, Status: 0,
		PostID: 500, AuthorID: 7, Revision: 3,
	}))
	assert.Equal(t, []bool{false, false, false, false}, recalled())
	features, err = env.Redis.HgetallCtx(ctx, "feature:v2-draft:post:500")
	require.NoError(t, err)
	assert.Equal(t, "unpublished", features["status"])
	assert.Equal(t, "3", features["revision"])
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
	store := NewRedisBehaviorStore(env.Redis, "v2-ordering", "recommend", 3600, enabledPreferences())
	candidates := NewRedisCandidateStore(env.Redis, "v2-ordering", "recommend", 3600, enabledPreferences())

	require.NoError(t, candidates.RecordPost(ctx, event.PostEvent{
		EventID: 201, EventTime: 1_000, Type: event.PostEventCreated, Status: 1,
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
