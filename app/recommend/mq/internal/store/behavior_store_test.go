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
	require.Len(t, redis.keys, 15)
	assert.Equal(t, "feature:v2:dedup:1", redis.keys[0])
	assert.Equal(t, "feature:v2:u:42:recent", redis.keys[1])
	assert.Equal(t, "recommend:v2:recall:post:hot:home", redis.keys[5])
	assert.Equal(t, "recommend:v2:recall:post:itemcf:u:42:home", redis.keys[6])
	assert.Equal(t, "feature:v2:u:42:state", redis.keys[14])
	assert.Equal(t, 3600, redis.args[0])
	assert.Equal(t, int64(100), redis.args[9])
	assert.Equal(t, "client-1", redis.args[10])
	var recent map[string]any
	require.NoError(t, json.Unmarshal([]byte(redis.args[1].(string)), &recent))
	assert.Equal(t, "client-1", recent["client_event_id"])
	assert.Equal(t, "request-1", recent["request_id"])
}

func TestRedisBehaviorStoreHashesAnonymousIdentity(t *testing.T) {
	redis := &fakeEvaler{}
	behavior := featureBehavior()
	behavior.UserID = 0
	behavior.AnonymousID = "device/unsafe:key"

	require.NoError(t, NewRedisBehaviorStore(redis, "v2", "recommend", 3600).Record(context.Background(), behavior))
	assert.Contains(t, redis.keys[1], "feature:v2:a:")
	assert.NotContains(t, redis.keys[1], behavior.AnonymousID)
}

func TestRedisBehaviorStorePropagatesRedisFailure(t *testing.T) {
	err := NewRedisBehaviorStore(&fakeEvaler{err: errors.New("redis down")}, "v2", "recommend", 3600).
		Record(context.Background(), featureBehavior())
	assert.ErrorContains(t, err, "redis down")
}
