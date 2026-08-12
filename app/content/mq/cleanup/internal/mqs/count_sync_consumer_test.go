package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/app/content/mq/cleanup/internal/store"
	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCountSyncStore struct {
	events []event.BehaviorEvent
	err    error
}

func (f *fakeCountSyncStore) ApplyBehaviorCount(_ context.Context, behavior event.BehaviorEvent) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, behavior)
	return nil
}

func behaviorMessage(t *testing.T, behavior event.BehaviorEvent) *primitive.MessageExt {
	t.Helper()
	payload, err := json.Marshal(behavior)
	require.NoError(t, err)
	return &primitive.MessageExt{Message: primitive.Message{Body: payload}}
}

func TestConsumeCountSyncBatchAppliesEachEvent(t *testing.T) {
	store := &fakeCountSyncStore{}
	behavior := event.BehaviorEvent{
		EventID: 1, ClientEventID: "c1", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store, behaviorMessage(t, behavior))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, store.events, 1)
	assert.Equal(t, int64(55), store.events[0].TargetID)
}

func TestConsumeCountSyncBatchRetriesOnStoreFailure(t *testing.T) {
	store := &fakeCountSyncStore{err: errors.New("db down")}
	behavior := event.BehaviorEvent{
		EventID: 2, ClientEventID: "c2", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store, behaviorMessage(t, behavior))
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestConsumeCountSyncBatchSkipsMalformedMessages(t *testing.T) {
	store := &fakeCountSyncStore{}
	msg := &primitive.MessageExt{Message: primitive.Message{Body: []byte("{not-json")}}
	result := consumeCountSyncBatch(context.Background(), store, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.events)
}

var _ store.CountSyncStore = (*fakeCountSyncStore)(nil)
