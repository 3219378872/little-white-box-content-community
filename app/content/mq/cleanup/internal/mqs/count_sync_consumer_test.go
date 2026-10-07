package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/app/content/mq/cleanup/internal/store"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCountSyncStore 记录收到的事件；stale 模拟旧序号，err 模拟存储失败。
type fakeCountSyncStore struct {
	events []event.BehaviorEvent
	stale  bool
	err    error
}

func (f *fakeCountSyncStore) ApplyBehaviorCount(_ context.Context, behavior event.BehaviorEvent) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.events = append(f.events, behavior)
	return !f.stale, nil
}

func behaviorMessage(t *testing.T, behavior event.BehaviorEvent) *primitive.MessageExt {
	t.Helper()
	payload, err := json.Marshal(behavior)
	require.NoError(t, err)
	return &primitive.MessageExt{Body: payload}
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

// 永久错误（如旧格式事件缺计数快照）确认丢弃，不阻塞同批后续消息。
func TestConsumeCountSyncBatchAcksPermanentStoreErrors(t *testing.T) {
	store := &fakeCountSyncStore{err: mqx.ErrPermanentEvent("count-sync: behavior event has no count snapshot")}
	behavior := event.BehaviorEvent{
		EventID: 8, ClientEventID: "c8", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store, behaviorMessage(t, behavior))
	assert.Equal(t, consumer.ConsumeSuccess, result)
}

// 旧序号或重复投递不推进投影，但仍确认消息。
func TestConsumeCountSyncBatchAcksStaleSnapshots(t *testing.T) {
	store := &fakeCountSyncStore{stale: true}
	behavior := event.BehaviorEvent{
		EventID: 9, ClientEventID: "c9", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionUnlike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store, behaviorMessage(t, behavior))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, store.events, 1)
}

func TestConsumeCountSyncBatchSkipsMalformedMessages(t *testing.T) {
	store := &fakeCountSyncStore{}
	msg := &primitive.MessageExt{Body: []byte("{not-json")}
	result := consumeCountSyncBatch(context.Background(), store, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.events)
}

var _ store.CountSyncStore = (*fakeCountSyncStore)(nil)
