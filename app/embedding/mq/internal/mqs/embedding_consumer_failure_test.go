package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/embedding/mq/internal/svc"
	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/stretchr/testify/assert"
)

func TestEmbeddingConsumer_InvalidEvent_Skips(t *testing.T) {
	store := newRecordingStore()
	// 缺少 event_id：Validate 失败，按无效消息跳过。
	e := event.PostEvent{EventTime: 9, Type: event.PostEventCreated, PostID: 1006, AuthorID: 42, Status: 1}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store,
		mq("m9", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Empty(t, store.upserted)
	assert.NotContains(t, store.deleted, int64(1006))
}

func TestEmbeddingConsumer_ReadRevisionError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.revErr = errors.New("milvus down")
	e := event.PostEvent{
		EventID: 10, EventTime: 10, Type: event.PostEventUpdated,
		PostID: 1007, AuthorID: 42, Title: "t", Status: 1,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store,
		mq("m10", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
	assert.Empty(t, store.upserted)
}

func TestEmbeddingConsumer_DeleteEventTypeError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.delErr = errors.New("milvus down")
	e := event.PostEvent{
		EventID: 11, EventTime: 11, Type: event.PostEventDeleted, PostID: 1008,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store,
		mq("m11", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestNewEmbeddingConsumerRejectsMissingNameServer(t *testing.T) {
	// MQ 配置缺失：构造消费者前即失败，不触达 RocketMQ。
	_, err := NewEmbeddingConsumer(&svc.ServiceContext{})
	assert.Error(t, err)
}
