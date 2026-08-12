package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEvaler struct {
	keys []string
	args []any
	err  error
}

func (f *fakeEvaler) EvalCtx(_ context.Context, _ string, keys []string, args ...any) (any, error) {
	f.keys = keys
	f.args = args
	return int64(1), f.err
}

type fakeGetterEvaler struct {
	*fakeEvaler
	getValues map[string]string
	getErrs   map[string]error
}

func (f *fakeGetterEvaler) GetCtx(_ context.Context, key string) (string, error) {
	if err := f.getErrs[key]; err != nil {
		return "", err
	}
	return f.getValues[key], nil
}

func featureBehavior() event.BehaviorEvent {
	return event.BehaviorEvent{
		EventID: 1, ClientEventID: "client-1", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 101, UserID: 42, Action: "like",
		TargetID: 9, TargetType: "post", Scene: "home", Producer: "behavior-rpc",
	}
}

func TestRedisBehaviorStoreUsesVersionedAtomicKeys(t *testing.T) {
	redis := &fakeEvaler{}
	store := NewRedisBehaviorStore(redis, "v2", "recommend", 3600)
	behavior := featureBehavior()
	behavior.RequestID = "request-1"

	require.NoError(t, store.Record(context.Background(), behavior))
	require.Len(t, redis.keys, 16)
	assert.Equal(t, "feature:v2:dedup:1", redis.keys[0])
	assert.Equal(t, "", redis.keys[1], "non-exposure event has no exposure dedup key")
	assert.Equal(t, "feature:v2:u:42:recent", redis.keys[2])
	assert.Equal(t, "recommend:v2:recall:post:hot:home", redis.keys[6])
	assert.Equal(t, "recommend:v2:recall:post:itemcf:u:42:home", redis.keys[7])
	assert.Equal(t, "feature:v2:u:42:state", redis.keys[15])
	assert.Equal(t, 3600, redis.args[0])
	assert.Equal(t, int64(100), redis.args[9])
	assert.Equal(t, "client-1", redis.args[10])
	var recent map[string]any
	require.NoError(t, json.Unmarshal([]byte(redis.args[1].(string)), &recent))
	assert.Equal(t, "client-1", recent["client_event_id"])
	assert.Equal(t, "request-1", recent["request_id"])
}

func TestRedisBehaviorStoreSkipsAnonymousFeatureRecording(t *testing.T) {
	redis := &fakeEvaler{}
	behavior := featureBehavior()
	behavior.UserID = 0
	behavior.AnonymousID = "device/unsafe:key"

	require.NoError(t, NewRedisBehaviorStore(redis, "v2", "recommend", 3600).Record(context.Background(), behavior))
	// DISC-031：匿名事件不进入推荐在线特征，不建立跨会话匿名画像。
	assert.Empty(t, redis.keys, "anonymous events must not write feature keys")
}

func TestRedisBehaviorStorePropagatesRedisFailure(t *testing.T) {
	err := NewRedisBehaviorStore(&fakeEvaler{err: errors.New("redis down")}, "v2", "recommend", 3600).
		Record(context.Background(), featureBehavior())
	assert.ErrorContains(t, err, "redis down")
}

func TestRedisBehaviorStoreSkipsAndPurgesFeaturesWhenOptedOut(t *testing.T) {
	evaler := &fakeEvaler{}
	store := NewRedisBehaviorStore(&fakeGetterEvaler{
		fakeEvaler: evaler,
		getValues:  map[string]string{"personalization:optout:42": "1"},
	}, "v2", "recommend", 3600)

	require.NoError(t, store.Record(context.Background(), featureBehavior()))

	// 只执行一次 purge DEL 脚本，不再记录行为特征
	require.Len(t, evaler.keys, 10)
	assert.Equal(t, "feature:v2:u:42:recent", evaler.keys[0])
	assert.Equal(t, "recommend:v2:recall:post:itemcf:u:42", evaler.keys[6])
}

func TestRedisBehaviorStoreStillRecordsWhenOptIn(t *testing.T) {
	evaler := &fakeEvaler{}
	store := NewRedisBehaviorStore(&fakeGetterEvaler{
		fakeEvaler: evaler,
		getValues:  map[string]string{"personalization:optout:42": ""},
	}, "v2", "recommend", 3600)

	require.NoError(t, store.Record(context.Background(), featureBehavior()))
	require.Len(t, evaler.keys, 16)
}

func TestRedisBehaviorStoreExposureCarriesRequestPostDedupKey(t *testing.T) {
	redis := &fakeEvaler{}
	store := NewRedisBehaviorStore(redis, "v2", "recommend", 3600)
	behavior := featureBehavior()
	behavior.Action = event.BehaviorActionExposure
	behavior.RequestID = "request-1"
	behavior.Position = int32Ptr(3)

	require.NoError(t, store.Record(context.Background(), behavior))
	require.Len(t, redis.keys, 16)
	// REL-004：同一 (requestId, postId) 最多记录一次曝光。
	assert.Equal(t, "feature:v2:u:42:exposure:dedup:request-1:9", redis.keys[1])
}

func int32Ptr(value int32) *int32 { return &value }
