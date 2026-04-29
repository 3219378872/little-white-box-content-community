package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/recommend/mq/internal/store"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

type recordingStore struct{ recorded []store.BehaviorEvent }

func (r *recordingStore) Record(ctx context.Context, e store.BehaviorEvent) error {
	r.recorded = append(r.recorded, e)
	return nil
}

type errorStore struct{ err error }

func (e *errorStore) Record(ctx context.Context, ev store.BehaviorEvent) error { return e.err }

func TestRecommendConsumer_MalformedJSON_Skips(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "msg-1"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.recorded)
}

func TestRecommendConsumer_MissingUserID_Skips(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"action":"view"}`)}, MsgId: "msg-2"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.recorded)
}

func TestRecommendConsumer_MissingAction_Skips(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"user_id":1}`)}, MsgId: "msg-3"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.recorded)
}

func TestRecommendConsumer_ValidEvent_Records(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"user_id":1,"action":"view","target_id":99,"target_type":"post"}`)}, MsgId: "msg-4"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.recorded, 1)
	assert.Equal(t, int64(1), rec.recorded[0].UserID)
	assert.Equal(t, "view", rec.recorded[0].Action)
}

func TestRecommendConsumer_StoreError_ReturnsRetry(t *testing.T) {
	errStore := &errorStore{err: errors.New("store offline")}
	result := consumeBehaviorBatch(context.Background(), errStore,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"user_id":1,"action":"view","target_id":99,"target_type":"post"}`)}, MsgId: "msg-5"},
	)
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}
