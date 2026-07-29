package embedder

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	embeddingpb "esx/app/embedding/mq/xiaobaihe/embedding/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

const testModelVersion = "test-model@abc123"

type fakeEmbeddingClient struct {
	healthResp *embeddingpb.EmbeddingHealthResp
	healthErr  error
	embedResp  *embeddingpb.EmbedResp
	embedErr   error
	batchResp  *embeddingpb.EmbedBatchResp
	batchErr   error
}

func (f *fakeEmbeddingClient) Health(context.Context, *embeddingpb.EmbeddingHealthReq, ...grpc.CallOption) (*embeddingpb.EmbeddingHealthResp, error) {
	return f.healthResp, f.healthErr
}

func (f *fakeEmbeddingClient) Embed(context.Context, *embeddingpb.EmbedReq, ...grpc.CallOption) (*embeddingpb.EmbedResp, error) {
	return f.embedResp, f.embedErr
}

func (f *fakeEmbeddingClient) EmbedBatch(context.Context, *embeddingpb.EmbedBatchReq, ...grpc.CallOption) (*embeddingpb.EmbedBatchResp, error) {
	return f.batchResp, f.batchErr
}

func testConfig() ClientConfig {
	return ClientConfig{
		ExpectedModelVersion: testModelVersion,
		ExpectedDimension:    3,
		Timeout:              time.Second,
		MaxTextBytes:         32,
		MaxBatchSize:         4,
		MaxBatchBytes:        64,
	}
}

func healthyFake() *fakeEmbeddingClient {
	return &fakeEmbeddingClient{
		healthResp: &embeddingpb.EmbeddingHealthResp{
			Ready: true, ModelVersion: testModelVersion, Dimension: 3,
		},
		embedResp: &embeddingpb.EmbedResp{
			Vector: []float32{0.1, 0.2, 0.3}, ModelVersion: testModelVersion, Dimension: 3,
		},
		batchResp: &embeddingpb.EmbedBatchResp{
			Items: []*embeddingpb.EmbedBatchItem{
				{Vector: []float32{0.1, 0.2, 0.3}},
				{Vector: []float32{0.4, 0.5, 0.6}},
			},
			ModelVersion: testModelVersion, Dimension: 3,
		},
	}
}

func TestGRPCEmbedderEmbedsValidatedVector(t *testing.T) {
	emb, err := NewGRPCEmbedderWithClient(context.Background(), healthyFake(), testConfig())
	require.NoError(t, err)

	result, err := emb.Embed(context.Background(), "hello")
	require.NoError(t, err)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, result.Vector)
	assert.Equal(t, testModelVersion, result.ModelVersion)
	assert.Equal(t, 3, result.Dimension)
}

func TestGRPCEmbedderEmbedsBatch(t *testing.T) {
	emb, err := NewGRPCEmbedderWithClient(context.Background(), healthyFake(), testConfig())
	require.NoError(t, err)

	results, err := emb.EmbedBatch(context.Background(), []string{"one", "two"})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, []float32{0.4, 0.5, 0.6}, results[1].Vector)
	assert.Equal(t, testModelVersion, results[1].ModelVersion)
}

func TestGRPCEmbedderRejectsUnhealthyOrDriftedModel(t *testing.T) {
	tests := []struct {
		name string
		fake *fakeEmbeddingClient
		want string
	}{
		{name: "RPC failure", fake: &fakeEmbeddingClient{healthErr: errors.New("unavailable")}, want: "unavailable"},
		{name: "not ready", fake: &fakeEmbeddingClient{healthResp: &embeddingpb.EmbeddingHealthResp{}}, want: "not ready"},
		{name: "wrong version", fake: &fakeEmbeddingClient{healthResp: &embeddingpb.EmbeddingHealthResp{Ready: true, ModelVersion: "other", Dimension: 3}}, want: "version mismatch"},
		{name: "wrong dimension", fake: &fakeEmbeddingClient{healthResp: &embeddingpb.EmbeddingHealthResp{Ready: true, ModelVersion: testModelVersion, Dimension: 4}}, want: "dimension metadata mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGRPCEmbedderWithClient(context.Background(), tt.fake, testConfig())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestNewGRPCEmbedderFailsWhenRequiredServiceIsUnavailable(t *testing.T) {
	cfg := testConfig()
	cfg.Address = "127.0.0.1:1"
	cfg.Timeout = 100 * time.Millisecond

	_, err := NewGRPCEmbedder(context.Background(), cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check")
}

func TestGRPCEmbedderRejectsInvalidVectors(t *testing.T) {
	tests := []struct {
		name   string
		vector []float32
		dim    int32
		want   string
	}{
		{name: "empty", vector: nil, dim: 3, want: "empty vector"},
		{name: "all zero", vector: []float32{0, 0, 0}, dim: 3, want: "all zero"},
		{name: "nan", vector: []float32{0.1, float32(math.NaN()), 0.3}, dim: 3, want: "non-finite"},
		{name: "infinite", vector: []float32{0.1, float32(math.Inf(1)), 0.3}, dim: 3, want: "non-finite"},
		{name: "length mismatch", vector: []float32{0.1, 0.2}, dim: 3, want: "vector dimension mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := healthyFake()
			fake.embedResp = &embeddingpb.EmbedResp{
				Vector: tt.vector, ModelVersion: testModelVersion, Dimension: tt.dim,
			}
			emb, err := NewGRPCEmbedderWithClient(context.Background(), fake, testConfig())
			require.NoError(t, err)
			_, err = emb.Embed(context.Background(), "hello")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestGRPCEmbedderRejectsInputLimitsAndBadBatchResponse(t *testing.T) {
	fake := healthyFake()
	emb, err := NewGRPCEmbedderWithClient(context.Background(), fake, testConfig())
	require.NoError(t, err)

	_, err = emb.Embed(context.Background(), "   ")
	require.ErrorContains(t, err, "blank")
	_, err = emb.Embed(context.Background(), "this text is far longer than the configured limit")
	require.ErrorContains(t, err, "byte limit")

	fake.batchResp.Items = fake.batchResp.Items[:1]
	_, err = emb.EmbedBatch(context.Background(), []string{"one", "two"})
	require.ErrorContains(t, err, "item count mismatch")
}
