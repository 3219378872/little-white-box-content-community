package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"esx/app/content/rpc/contentservice"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/event"

	"github.com/cloudwego/kitex/client/callopt"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingStore struct {
	mu       sync.Mutex
	upserted map[int64]vectorstore.Record
	deleted  map[int64]struct{}
	revs     map[int64]int64
	upErr    error
	delErr   error
	revErr   error
}

func newRecordingStore() *recordingStore {
	return &recordingStore{
		upserted: map[int64]vectorstore.Record{},
		deleted:  map[int64]struct{}{},
		revs:     map[int64]int64{},
	}
}

func (r *recordingStore) Upsert(_ context.Context, record vectorstore.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.upErr != nil {
		return r.upErr
	}
	if record.Revision < r.revs[record.PostID] {
		return nil
	}
	if record.Revision == r.revs[record.PostID] {
		if _, deleted := r.deleted[record.PostID]; deleted {
			return nil
		}
	}
	r.upserted[record.PostID] = record
	r.revs[record.PostID] = record.Revision
	delete(r.deleted, record.PostID)
	return nil
}

func (r *recordingStore) Delete(_ context.Context, postID, revision int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.delErr != nil {
		return r.delErr
	}
	if revision < r.revs[postID] {
		return nil
	}
	r.revs[postID] = revision
	r.deleted[postID] = struct{}{}
	delete(r.upserted, postID)
	return nil
}

func (r *recordingStore) CurrentRevision(_ context.Context, postID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.revErr != nil {
		return 0, r.revErr
	}
	return r.revs[postID], nil
}

type errorEmbedder struct{ err error }

func (e errorEmbedder) Embed(_ context.Context, _ string) (embedder.Embedding, error) {
	return embedder.Embedding{}, e.err
}

type fixedEmbedder struct{}

func (fixedEmbedder) Embed(_ context.Context, _ string) (embedder.Embedding, error) {
	return embedder.Embedding{
		Vector:       []float32{0.1, 0.2, 0.3},
		ModelVersion: "test-model@v1",
		Dimension:    3,
	}, nil
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func mq(id string, body []byte) *primitive.MessageExt {
	return &primitive.MessageExt{Body: body, MsgId: id}
}

func TestEmbeddingConsumer_PostCreated_UpsertsVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 1, EventTime: 1, Type: event.PostEventCreated,
		PostID: 999, AuthorID: 42, Title: "hello", Status: 1,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(e), mq("m1", mustMarshal(t, e)))
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
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(e), mq("m2", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.upserted, int64(1000))
}

func TestEmbeddingConsumer_PostDeleted_DeletesVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 3, EventTime: 3, Type: event.PostEventDeleted, PostID: 1001,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(e), mq("m3", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.deleted, int64(1001))
}

func TestEmbeddingConsumer_InvalidJSON_Skips(t *testing.T) {
	store := newRecordingStore()
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(event.PostEvent{}), mq("m4", []byte(`bad`)))
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
		errorEmbedder{err: errors.New("model unavailable")}, store, contentFor(e), mq("m5", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_UpsertError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.upErr = errors.New("milvus down")
	e := event.PostEvent{
		EventID: 6, EventTime: 6, Type: event.PostEventCreated,
		PostID: 1003, AuthorID: 42, Status: 1,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(e), mq("m6", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_DeleteError_ReturnsRetry(t *testing.T) {
	store := newRecordingStore()
	store.delErr = errors.New("milvus down")
	e := event.PostEvent{
		EventID: 7, EventTime: 7, Type: event.PostEventCreated,
		PostID: 1004, AuthorID: 42, Status: 0,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(e), mq("m7", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestEmbeddingConsumer_NonPublished_RemovesVector(t *testing.T) {
	store := newRecordingStore()
	e := event.PostEvent{
		EventID: 8, EventTime: 8, Type: event.PostEventUpdated,
		PostID: 1005, AuthorID: 42, Status: 0,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(e), mq("m8", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Contains(t, store.deleted, int64(1005))
	assert.NotContains(t, store.upserted, int64(1005))
}

func TestEmbeddingConsumer_StaleUpdateAfterNewerDoesNotOverwrite(t *testing.T) {
	store := newRecordingStore()
	newer := event.PostEvent{
		EventID: 20, EventTime: 20, Type: event.PostEventUpdated,
		PostID: 2001, AuthorID: 42, Title: "C", Status: 1, Revision: 3,
	}
	older := event.PostEvent{
		EventID: 10, EventTime: 10, Type: event.PostEventUpdated,
		PostID: 2001, AuthorID: 42, Title: "B", Status: 1, Revision: 2,
	}
	res := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, contentFor(newer),
		mq("c", mustMarshal(t, newer)), mq("b", mustMarshal(t, older)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	require.Contains(t, store.upserted, int64(2001))
	assert.Equal(t, int64(3), store.upserted[2001].Revision)
	assert.Equal(t, int64(3), store.revs[2001])
}

type testContentSource struct {
	post *contentservice.PostInfo
	err  error
}

func (s testContentSource) GetPostsByIds(context.Context, *contentservice.GetPostsByIdsReq, ...callopt.Option) (*contentservice.GetPostsByIdsResp, error) {
	if s.err != nil {
		return nil, s.err
	}
	resp := &contentservice.GetPostsByIdsResp{}
	if s.post != nil {
		resp.Posts = []*contentservice.PostInfo{s.post}
	}
	return resp, nil
}
func contentFor(e event.PostEvent) testContentSource {
	if e.Type == event.PostEventDeleted || e.Status != 1 {
		return testContentSource{}
	}
	return testContentSource{post: &contentservice.PostInfo{Id: e.PostID, Revision: max(e.Revision, 1), Status: 1, Title: e.Title, Content: e.IndexText()}}
}

func TestEmbeddingConsumerRebuildOldPublishedReplayKeepsAbsentPostDeleted(t *testing.T) {
	store := newRecordingStore() // no version/tombstone survived the new collection
	old := event.PostEvent{EventID: 1, EventTime: 1, Type: event.PostEventUpdated, PostID: 91, AuthorID: 7, Status: 1, Revision: 2}
	result := consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, testContentSource{}, mq("old", mustMarshal(t, old)))
	require.Equal(t, consumer.ConsumeSuccess, result)
	require.Contains(t, store.deleted, int64(91))
	require.Equal(t, int64(2), store.revs[91])
}
func TestEmbeddingConsumerAuthorityFailureAndNewerAuthoritativeContent(t *testing.T) {
	for _, source := range []testContentSource{{err: errors.New("Content unavailable")}, {post: &contentservice.PostInfo{Id: 91, Status: 1, Revision: 1}}} {
		store := newRecordingStore()
		e := event.PostEvent{EventID: 1, EventTime: 1, Type: event.PostEventUpdated, PostID: 91, AuthorID: 7, Status: 1, Revision: 2}
		require.Equal(t, consumer.ConsumeRetryLater, consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, source, mq("old", mustMarshal(t, e))))
		require.Empty(t, store.upserted)
		require.Empty(t, store.deleted)
	}
	store := newRecordingStore()
	e := event.PostEvent{EventID: 1, EventTime: 1, Type: event.PostEventDeleted, PostID: 91, Revision: 2}
	source := testContentSource{post: &contentservice.PostInfo{Id: 91, Status: 1, Revision: 4, Title: "current"}}
	require.Equal(t, consumer.ConsumeSuccess, consumeEmbeddingBatch(context.Background(), fixedEmbedder{}, store, source, mq("old-delete", mustMarshal(t, e))))
	require.Equal(t, int64(4), store.revs[91])
	require.Empty(t, store.deleted)
}
