package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"esx/app/embedding/mq/internal/embedder"
	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingStore struct {
	mu       sync.Mutex
	upserted map[int64][]float32
	deleted  map[int64]struct{}
	upErr    error
	delErr   error
}

func newRecordingStore() *recordingStore {
	return &recordingStore{
		upserted: map[int64][]float32{},
		deleted:  map[int64]struct{}{},
	}
}

func (r *recordingStore) Upsert(_ context.Context, postID int64, vec []float32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upErr != nil {
		return r.upErr
	}
	r.upserted[postID] = vec
	return nil
}

func (r *recordingStore) Delete(_ context.Context, postID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.delErr != nil {
		return r.delErr
	}
	r.deleted[postID] = struct{}{}
	return nil
}

type errorEmbedder struct{ err error }

func (e errorEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, e.err
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func mq(id string, body []byte) *primitive.MessageExt {
	return &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: id}
}

func TestEmbeddingConsumer_PostCreated_UpsertsVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 1, EventTime: 1, Type: event.PostEventCreated,
		PostID: 999, AuthorID: 42, Title: "hello",
	}
	res := consumeEmbeddingBatch(context.Background(), embedder.NoopEmbedder{}, store, mq("m1", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	require.Contains(t, store.upserted, int64(999))
	assert.Len(t, store.upserted[999], embedder.EmbeddingDim)
}

func TestEmbeddingConsumer_PostUpdated_UpsertsVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 2, EventTime: 2, Type: event.PostEventUpdated,
		PostID: 1000, AuthorID: 42, Title: "world",
	}
	res := consumeEmbeddingBatch(context.Background(), embedder.NoopEmbedder{}, store, mq("m2", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.upserted, int64(1000))
}

func TestEmbeddingConsumer_PostDeleted_DeletesVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 3, EventTime: 3, Type: event.PostEventDeleted, PostID: 1001,
	}
	res := consumeEmbeddingBatch(context.Background(), embedder.NoopEmbedder{}, store, mq("m3", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.deleted, int64(1001))
}

func TestEmbeddingConsumer_InvalidJSON_Skips(t *testing.T) {
	store := newRecordingStore()
	res := consumeEmbeddingBatch(context.Background(), embedder.NoopEmbedder{}, store, mq("m4", []byte(`bad`)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Empty(t, store.upserted)
}

func TestEmbeddingConsumer_EmbedError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 5, EventTime: 5, Type: event.PostEventCreated,
		PostID: 1002, AuthorID: 42,
	}
	res := consumeEmbeddingBatch(context.Background(),
		errorEmbedder{err: errors.New("model unavailable")}, store, mq("m5", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_UpsertError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.upErr = errors.New("milvus down")
	e := event.PostEvent{
		EventID: 6, EventTime: 6, Type: event.PostEventCreated,
		PostID: 1003, AuthorID: 42,
	}
	res := consumeEmbeddingBatch(context.Background(), embedder.NoopEmbedder{}, store, mq("m6", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_DeleteError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.delErr = errors.New("milvus timeout")
	e := event.PostEvent{
		EventID: 7, EventTime: 7, Type: event.PostEventDeleted, PostID: 1004,
	}
	res := consumeEmbeddingBatch(context.Background(), embedder.NoopEmbedder{}, store, mq("m7", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}
