package mqs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"esx/pkg/event"
)

type fakeStorage struct {
	deleted []string
	err     error
}

func (s *fakeStorage) Delete(ctx context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *fakeStorage) BuildPublicURL(key string) string { return "http://fake/" + key }

func TestMediaCleanupConsumer_MalformedJSON_Skips(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Body: []byte(`not-json`), MsgId: "msg-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.deleted)
}

func TestMediaCleanupConsumer_EmptyObjectKey_Skips(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Body: []byte(`{"media_id":1,"s3_object_key":"","bucket":"xbh-media"}`), MsgId: "msg-2"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.deleted)
}

func TestMediaCleanupConsumer_ValidMessage_DeletesObject(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Body: []byte(`{"media_id":1,"s3_object_key":"obj/key","bucket":"xbh-media","deleted_at":1710000000}`), MsgId: "msg-3"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, store.deleted, 1)
	assert.Equal(t, "obj/key", store.deleted[0])
}

func TestMediaCleanupConsumer_DeleteFails_ReturnsRetry(t *testing.T) {
	store := &fakeStorage{err: errors.New("s3 unavailable")}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Body: []byte(`{"media_id":1,"s3_object_key":"obj/key","bucket":"xbh-media","deleted_at":1710000000}`), MsgId: "msg-4"},
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestMediaCleanupConsumer_BatchSkipsBadAndProcessesGood(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Body: []byte(`bad`), MsgId: "bad"},
		&primitive.MessageExt{Body: []byte(`{"media_id":2,"s3_object_key":"obj/key2","bucket":"xbh-media","deleted_at":1710000000}`), MsgId: "good"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.deleted, 1)
	assert.Equal(t, "obj/key2", store.deleted[0])
}

func TestMediaLagSeconds_LegacyDeletedAtSecondsYieldsRealLag(t *testing.T) {
	now := time.UnixMilli(1_710_000_010_000)

	lag := mediaLagSeconds(event.MediaDeletedEvent{DeletedAt: 1_710_000_000}, now)

	assert.InDelta(t, 10, lag, 0.001)
}

func TestMediaLagSeconds_PrefersMillisecondEventTime(t *testing.T) {
	now := time.UnixMilli(1_710_000_010_000)

	lag := mediaLagSeconds(event.MediaDeletedEvent{
		EventTime: 1_710_000_007_500, DeletedAt: 1_710_000_000,
	}, now)

	assert.InDelta(t, 2.5, lag, 0.001)
}

func TestMediaLagSeconds_MissingTimeRecordsZeroLag(t *testing.T) {
	assert.Zero(t, mediaLagSeconds(event.MediaDeletedEvent{}, time.Now()))
}
