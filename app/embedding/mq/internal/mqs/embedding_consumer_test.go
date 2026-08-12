package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingStore struct {
	mu       sync.Mutex
	upserted map[int64]vectorstore.Record
	deleted  map[int64]struct{}
	upErr    error
	delErr   error
}

func newRecordingStore() *recordingStore {
	return &recordingStore{
		upserted: map[int64]vectorstore.Record{},
		deleted:  map[int64]struct{}{},
	}
}

func (r *recordingStore) Upsert(_ context.Context, record vectorstore.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upErr != nil {
		return r.upErr
	}
	r.upserted[record.PostID] = record
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

func (e errorEmbedder) Embed(_ context.Context, _ string) (embedder.Embedding, error) {
	return embedder.Embedding{}, e.err
}

type fixedEmbedder struct{}

func (fixedEmbedder) Embed(_ context.Context, _ string) (embedder.Embedding, error) {
	return embedder.Embedding{
		Vector: []float32{0.1, 0.2, 0.3},
		Metadata: embedder.Metadata{
			ModelVersion: "test-model@v1",
			Dimension:    3,
		},
	}, nil
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
		PostID: 999, AuthorID: 42, Title: "hello", Status: 1,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m1", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	require.Contains(t, store.upserted, int64(999))
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, store.upserted[999].Vector)
	assert.Equal(t, "test-model@v1", store.upserted[999].ModelVersion)
	assert.Equal(t, 3, store.upserted[999].Dimension)
}

func TestEmbeddingConsumer_PostUpdated_UpsertsVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 2, EventTime: 2, Type: event.PostEventUpdated,
		PostID: 1000, AuthorID: 42, Title: "world", Status: 1,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m2", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.upserted, int64(1000))
}

func TestEmbeddingConsumer_PostDeleted_DeletesVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 3, EventTime: 3, Type: event.PostEventDeleted, PostID: 1001,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m3", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.deleted, int64(1001))
}

func TestEmbeddingConsumer_InvalidJSON_Skips(t *testing.T) {
	store := newRecordingStore()
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m4", []byte(`bad`)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Empty(t, store.upserted)
}

func TestEmbeddingConsumer_EmbedError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 5, EventTime: 5, Type: event.PostEventCreated,
		PostID: 1002, AuthorID: 42, Status: 1,
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
		PostID: 1003, AuthorID: 42, Status: 1,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m6", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_DeleteError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.delErr = errors.New("milvus down")
	e := event.PostEvent{
		EventID: 7, EventTime: 7, Type: event.PostEventCreated,
		PostID: 1004, AuthorID: 42, Status: 0,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m7", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_NonPublished_RemovesVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 8, EventTime: 8, Type: event.PostEventUpdated,
		PostID: 1005, AuthorID: 42, Status: 0,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, mq("m8", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.deleted, int64(1005))
	assert.NotContains(t, store.upserted, int64(1005))
}
