package rebuild

import (
	"context"
	"errors"
	"testing"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeSource struct {
	// pages 以请求游标为键（"" 为首页）；缺键视为空页（终止翻页）。
	pages    map[string]*contentservice.GetPostListResp
	failures int
	calls    int
}

func (f *fakeSource) GetPostList(_ context.Context, req *contentservice.GetPostListReq, _ ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, errors.New("content unavailable")
	}
	if resp, ok := f.pages[req.Cursor]; ok {
		return resp, nil
	}
	return &contentservice.GetPostListResp{}, nil
}

type fakeBatchEmbedder struct {
	failures int
	calls    int
}

func (f *fakeBatchEmbedder) Embed(context.Context, string) (embedder.Embedding, error) {
	return embedder.Embedding{}, errors.New("unused")
}

func (f *fakeBatchEmbedder) EmbedBatch(_ context.Context, texts []string) ([]embedder.Embedding, error) {
	f.calls++
	if f.calls <= f.failures {
		return nil, errors.New("model unavailable")
	}
	results := make([]embedder.Embedding, len(texts))
	for i := range results {
		results[i] = embedder.Embedding{
			Vector:       []float32{float32(i) + 0.1, 0.2},
			ModelVersion: "model@sha", Dimension: 2,
		}
	}
	return results, nil
}

type fakeTarget struct {
	failures   int
	upCalls    int
	count      int64
	flushed    bool
	promoted   string
	promoteErr error
	records    []vectorstore.Record
}

func (f *fakeTarget) UpsertBatch(_ context.Context, records []vectorstore.Record) error {
	f.upCalls++
	if f.upCalls <= f.failures {
		return errors.New("Milvus unavailable")
	}
	f.records = append(f.records, records...)
	f.count = int64(len(f.records))
	return nil
}

func (f *fakeTarget) Flush(context.Context) error {
	f.flushed = true
	return nil
}

func (f *fakeTarget) Count(context.Context) (int64, error) { return f.count, nil }

func (f *fakeTarget) PromoteAlias(_ context.Context, alias string) error {
	if f.promoteErr != nil {
		return f.promoteErr
	}
	f.promoted = alias
	return nil
}

func testOptions() Options {
	return Options{PageSize: 2, BatchSize: 2, MaxAttempts: 3, RetryBackoff: time.Millisecond}
}

func TestRunAndPromoteRetriesAndIndexesOnlyPublishedPosts(t *testing.T) {
	source := &fakeSource{
		failures: 1,
		pages: map[string]*contentservice.GetPostListResp{
			"": {NextCursor: "c2", Posts: []*contentservice.PostInfo{
				{Id: 1, Title: "one", Content: "body", Status: 1},
				{Id: 2, Status: 0},
			}},
			"c2": {Posts: []*contentservice.PostInfo{{Id: 3, Title: "three", Status: 1}}},
		},
	}
	emb := &fakeBatchEmbedder{failures: 1}
	target := &fakeTarget{failures: 1}

	count, err := RunAndPromote(context.Background(), source, emb, target, "post_embeddings_current", testOptions())
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.Len(t, target.records, 2)
	assert.True(t, target.flushed)
	assert.Equal(t, "post_embeddings_current", target.promoted)
	assert.GreaterOrEqual(t, source.calls, 3)
	assert.GreaterOrEqual(t, emb.calls, 3)
	assert.GreaterOrEqual(t, target.upCalls, 3)
}

func TestRunAndPromoteDoesNotPromotePartialBuild(t *testing.T) {
	source := &fakeSource{pages: map[string]*contentservice.GetPostListResp{
		"": {Posts: []*contentservice.PostInfo{{Id: 1, Title: "one", Status: 1}}},
	}}
	target := &fakeTarget{failures: 10}

	count, err := RunAndPromote(context.Background(), source, &fakeBatchEmbedder{}, target, "active", testOptions())
	require.ErrorContains(t, err, "failed after 3 attempts")
	assert.Zero(t, count)
	assert.Empty(t, target.promoted)
}

func TestRunAndPromoteRejectsCountMismatch(t *testing.T) {
	source := &fakeSource{pages: map[string]*contentservice.GetPostListResp{
		"": {Posts: []*contentservice.PostInfo{{Id: 1, Title: "one", Status: 1}}},
	}}
	target := &fakeTarget{}
	target.count = 99
	// Keep the fake count wrong after successful upsert.
	target.promoteErr = errors.New("not reached")

	// A wrapper forces the post-flush count mismatch while retaining writes.
	mismatch := &countMismatchTarget{fakeTarget: target, count: 99}
	_, err := RunAndPromote(context.Background(), source, &fakeBatchEmbedder{}, mismatch, "active", testOptions())
	require.ErrorContains(t, err, "row count mismatch")
	assert.Empty(t, target.promoted)
}

type countMismatchTarget struct {
	*fakeTarget
	count int64
}

func (f *countMismatchTarget) Count(context.Context) (int64, error) { return f.count, nil }

func TestVersionedCollectionNameIsMilvusSafeAndTraceable(t *testing.T) {
	name, err := VersionedCollectionName("post-embeddings", "multilingual/model@abc123", time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, "post_embeddings_multilingual_model_abc123_20260729_010203_000000000", name)
}
