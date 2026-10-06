// Package router 实现机审召回（Router）：既有 embedding 服务把文案向量化，在 Milvus 种子集合中
// 按市场检索 top-k，再按权威库复核种子状态（DES-review-platform「机审级联」「种子库」）。
package router

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	embeddingpb "esx/app/embedding/mq/xiaobaihe/embedding/pb"
	"esx/app/review/internal/cascade"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
)

// Embedder 把文本转为向量。
type Embedder interface {
	Embed(ctx context.Context, text string) (vector []float32, modelVersion string, err error)
}

// Hit 是一次种子检索命中。
type Hit struct {
	SeedID int64
	Issue  string
	Score  float64
}

// Index 是种子向量集合。
type Index interface {
	Search(ctx context.Context, vector []float32, market string, topK int) ([]Hit, error)
}

// Authority 是种子的权威状态来源（xbh_review）。
type Authority interface {
	SeedStatuses(ctx context.Context, ids []int64) (map[int64]string, error)
	ActiveSeedSummary(ctx context.Context, market string) (count int64, latest int64, err error)
}

// Router 实现 cascade.RouterWithVersion。
type Router struct {
	Embedder  Embedder
	Index     Index
	Authority Authority
	TopK      int
	version   string
}

// Version 返回路由组件版本，写入阶段记录便于追溯。
func (r *Router) Version() string {
	if r.version != "" {
		return r.version
	}
	return "seedbank@unknown"
}

// Route 返回每个 issue 的最高相似度。市场没有生效种子时 Covered=false，级联据此对全部 issue 精排；
// 已停用种子即使仍在集合中也不计入（停用后新任务不再命中，RVW-030）。
func (r *Router) Route(ctx context.Context, in cascade.RouteInput) (cascade.RouteResult, error) {
	if r == nil || r.Embedder == nil || r.Index == nil || r.Authority == nil {
		return cascade.RouteResult{}, cascade.ErrUnavailable
	}
	count, latest, err := r.Authority.ActiveSeedSummary(ctx, in.Market)
	if err != nil {
		return cascade.RouteResult{}, fmt.Errorf("router: seed summary: %w", cascade.ErrUnavailable)
	}
	vector, modelVersion, err := r.Embedder.Embed(ctx, in.Text)
	if err != nil {
		return cascade.RouteResult{}, err
	}
	version := "seedbank@" + strconv.FormatInt(count, 10) + "." + strconv.FormatInt(latest, 10) + "+" + modelVersion
	r.version = version
	if count == 0 {
		return cascade.RouteResult{Version: version, Covered: false}, nil
	}
	topK := r.TopK
	if topK <= 0 {
		topK = 5
	}
	hits, err := r.Index.Search(ctx, normalize(vector), in.Market, topK)
	if err != nil {
		return cascade.RouteResult{}, fmt.Errorf("router: search: %w", cascade.ErrUnavailable)
	}
	ids := make([]int64, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.SeedID)
	}
	statuses, err := r.Authority.SeedStatuses(ctx, ids)
	if err != nil {
		return cascade.RouteResult{}, fmt.Errorf("router: seed statuses: %w", cascade.ErrUnavailable)
	}
	result := cascade.RouteResult{
		Version: version, Covered: true,
		Similarity: map[string]float64{}, SeedIDs: map[string][]int64{},
	}
	for _, hit := range hits {
		if statuses[hit.SeedID] != "active" {
			continue
		}
		if hit.Score > result.Similarity[hit.Issue] {
			result.Similarity[hit.Issue] = hit.Score
		}
		result.SeedIDs[hit.Issue] = append(result.SeedIDs[hit.Issue], hit.SeedID)
	}
	return result, nil
}

// normalize 让内积等价于余弦相似度。
func normalize(vector []float32) []float32 {
	var sum float64
	for _, v := range vector {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vector
	}
	norm := float32(math.Sqrt(sum))
	out := make([]float32, len(vector))
	for i, v := range vector {
		out[i] = v / norm
	}
	return out
}

// GRPCEmbedder 调用既有 embedding 服务（proto/embedding/embedding.proto）。
type GRPCEmbedder struct {
	conn   *grpc.ClientConn
	client embeddingpb.EmbeddingServiceClient
	dim    int
}

// DialEmbedder 建立到向量侧车的 gRPC 连接；连接是惰性的，侧车不可用不阻塞启动。
func DialEmbedder(address string, dim int) (*GRPCEmbedder, error) {
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("router: embedding address is required")
	}
	// 侧车晚于 worker 启动或重启时快速恢复；不可用期间调用按降级处理（RVW-011）。
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(),
		grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.Config{
			BaseDelay: 200 * time.Millisecond, Multiplier: 1.6, Jitter: 0.2, MaxDelay: 5 * time.Second,
		}, MinConnectTimeout: 2 * time.Second}))
	if err != nil {
		return nil, fmt.Errorf("router: dial embedding: %w", err)
	}
	return &GRPCEmbedder{conn: conn, client: embeddingpb.NewEmbeddingServiceClient(conn), dim: dim}, nil
}

// Close 关闭与向量侧车的连接。
func (e *GRPCEmbedder) Close() error {
	if e == nil || e.conn == nil {
		return nil
	}
	return e.conn.Close()
}

// Embed 生成文本向量；除超时外的失败与维度不符都归为 ErrUnavailable，由级联按降级处理。
func (e *GRPCEmbedder) Embed(ctx context.Context, text string) ([]float32, string, error) {
	resp, err := e.client.Embed(ctx, &embeddingpb.EmbedReq{Text: text})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("router: embed: %w", cascade.ErrUnavailable)
	}
	if len(resp.GetVector()) != e.dim || resp.GetModelVersion() == "" {
		return nil, "", fmt.Errorf("router: embedding dimension %d, want %d: %w", len(resp.GetVector()), e.dim, cascade.ErrUnavailable)
	}
	return resp.GetVector(), resp.GetModelVersion(), nil
}
