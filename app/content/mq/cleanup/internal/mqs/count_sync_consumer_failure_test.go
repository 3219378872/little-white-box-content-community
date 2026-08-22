package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/content/mq/cleanup/internal/svc"
	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsumeCountSyncBatchSkipsInvalidEvent(t *testing.T) {
	store := &fakeCountSyncStore{}
	// 缺少 client_event_id：Validate 失败，按无效消息跳过。
	behavior := event.BehaviorEvent{
		EventID:       3,
		SchemaVersion: event.BehaviorSchemaVersion,
		EventTime:     100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store, behaviorMessage(t, behavior))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.events)
}

func TestConsumeCountSyncBatchMixedBatchContinuesAfterInvalid(t *testing.T) {
	store := &fakeCountSyncStore{}
	valid := event.BehaviorEvent{
		EventID: 4, ClientEventID: "c4", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	invalid := event.BehaviorEvent{
		EventID:       5,
		SchemaVersion: event.BehaviorSchemaVersion,
		EventTime:     100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 56, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store,
		behaviorMessage(t, invalid), behaviorMessage(t, valid))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, store.events, 1)
	assert.Equal(t, int64(55), store.events[0].TargetID)
}

func TestConsumeCountSyncBatchRetryAbortsRemainingMessages(t *testing.T) {
	store := &fakeCountSyncStore{err: errors.New("db down")}
	first := event.BehaviorEvent{
		EventID: 6, ClientEventID: "c6", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 55, TargetType: "post", Producer: "business-outbox",
	}
	second := event.BehaviorEvent{
		EventID: 7, ClientEventID: "c7", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 8, Action: event.BehaviorActionUnlike,
		TargetID: 56, TargetType: "post", Producer: "business-outbox",
	}
	result := consumeCountSyncBatch(context.Background(), store,
		behaviorMessage(t, first), behaviorMessage(t, second))
	// 首条失败即整批重试，后续消息不得被应用。
	assert.Equal(t, consumer.ConsumeRetryLater, result)
	assert.Empty(t, store.events)
}

func TestNewCountSyncConsumerRejectsMissingGroup(t *testing.T) {
	// GroupName 缺失：构造消费者前即失败，不触达 MQ。
	_, err := NewCountSyncConsumer(&svc.ServiceContext{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GroupName is required")
}
