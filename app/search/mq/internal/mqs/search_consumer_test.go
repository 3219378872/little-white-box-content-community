package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/app/search/mq/internal/indexer"
	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errorIndexer struct{ err error }

func (e *errorIndexer) Index(ctx context.Context, doc indexer.IndexDoc) error { return e.err }
func (e *errorIndexer) Delete(ctx context.Context, docID string) error        { return e.err }

type recordingIndexer struct {
	indexed []indexer.IndexDoc
	deleted []string
}

func (r *recordingIndexer) Index(ctx context.Context, doc indexer.IndexDoc) error {
	r.indexed = append(r.indexed, doc)
	return nil
}
func (r *recordingIndexer) Delete(ctx context.Context, docID string) error {
	r.deleted = append(r.deleted, docID)
	return nil
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func msg(id string, body []byte) *primitive.MessageExt {
	return &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: id}
}

func TestSearchConsumer_MalformedJSON_Skips(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec, msg("m1", []byte(`bad`)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.indexed)
}

func TestSearchConsumer_PostCreated_IndexesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	e := event.PostEvent{
		EventID: 1, EventTime: 100, Type: event.PostEventCreated,
		PostID: 999, AuthorID: 42, Title: "hello", BodyExcerpt: "world",
		Tags: []string{"tag-1"},
	}
	result := consumeSearchBatch(context.Background(), rec, msg("m2", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, rec.indexed, 1)
	assert.Equal(t, "999", rec.indexed[0].DocID)
	assert.Equal(t, "hello", rec.indexed[0].Body["title"])
}

func TestSearchConsumer_PostUpdated_IndexesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	e := event.PostEvent{
		EventID: 2, EventTime: 200, Type: event.PostEventUpdated,
		PostID: 1000, AuthorID: 42, Title: "updated",
	}
	result := consumeSearchBatch(context.Background(), rec, msg("m3", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, rec.indexed, 1)
	assert.Equal(t, "1000", rec.indexed[0].DocID)
}

func TestSearchConsumer_PostDeleted_DeletesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	e := event.PostEvent{
		EventID: 3, EventTime: 300, Type: event.PostEventDeleted,
		PostID: 1001,
	}
	result := consumeSearchBatch(context.Background(), rec, msg("m4", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, rec.deleted, 1)
	assert.Equal(t, "1001", rec.deleted[0])
}

func TestSearchConsumer_InvalidEvent_Skips(t *testing.T) {
	rec := &recordingIndexer{}
	e := event.PostEvent{EventID: 0, Type: event.PostEventCreated, PostID: 1, AuthorID: 1}
	result := consumeSearchBatch(context.Background(), rec, msg("m5", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.indexed)
}

func TestSearchConsumer_IndexerError_ReturnsRetry(t *testing.T) {
	errIdx := &errorIndexer{err: errors.New("es unavailable")}
	e := event.PostEvent{
		EventID: 4, EventTime: 400, Type: event.PostEventCreated,
		PostID: 1002, AuthorID: 42,
	}
	result := consumeSearchBatch(context.Background(), errIdx, msg("m6", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestSearchConsumer_DeleteError_ReturnsRetry(t *testing.T) {
	errIdx := &errorIndexer{err: errors.New("es timeout")}
	e := event.PostEvent{
		EventID: 5, EventTime: 500, Type: event.PostEventDeleted, PostID: 1003,
	}
	result := consumeSearchBatch(context.Background(), errIdx, msg("m7", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}
