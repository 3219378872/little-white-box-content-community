package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/search/mq/internal/indexer"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

type errorIndexer struct{ err error }

func (e *errorIndexer) Index(ctx context.Context, doc indexer.IndexDoc) error {
	return e.err
}

func (e *errorIndexer) Delete(ctx context.Context, docID string) error {
	return e.err
}

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

func TestSearchConsumer_MalformedJSON_Skips(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "msg-1"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.indexed)
}

func TestSearchConsumer_SearchIndexEvent_IndexesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"index","doc_id":"doc-1","body":{"title":"hello"}}`)}, MsgId: "msg-2"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.indexed, 1)
	assert.Equal(t, "doc-1", rec.indexed[0].DocID)
}

func TestSearchConsumer_SearchDeleteEvent_DeletesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"delete","doc_id":"doc-2"}`)}, MsgId: "msg-3"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.deleted, 1)
	assert.Equal(t, "doc-2", rec.deleted[0])
}

func TestSearchConsumer_MissingDocID_Skips(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"index"}`)}, MsgId: "msg-4"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.indexed)
}

func TestSearchConsumer_IndexerError_ReturnsRetry(t *testing.T) {
	errIndexer := &errorIndexer{err: errors.New("es unavailable")}
	result := consumeSearchBatch(context.Background(), errIndexer,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"index","doc_id":"doc-3","body":{}}`)}, MsgId: "msg-5"},
	)
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}
