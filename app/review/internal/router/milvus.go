package router

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/grpc"
)

// MilvusIndex 是种子向量集合。权威状态在 xbh_review；集合只做检索加速，可由同步任务重建。
type MilvusIndex struct {
	cli        client.Client
	collection string
	dim        int
}

// MilvusConfig 是集合连接参数。
type MilvusConfig struct {
	Address    string
	Username   string
	Password   string
	Collection string
	Dim        int
}

// DialMilvus 连接并确保集合存在（内积度量，配合归一化向量即余弦相似度）。
func DialMilvus(ctx context.Context, cfg MilvusConfig) (*MilvusIndex, error) {
	if strings.TrimSpace(cfg.Address) == "" || strings.TrimSpace(cfg.Collection) == "" || cfg.Dim <= 0 {
		return nil, fmt.Errorf("router: milvus address, collection and dim are required")
	}
	connectCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var cli client.Client
	err := retryStartup(connectCtx, func() error {
		var dialErr error
		cli, dialErr = client.NewClient(connectCtx, client.Config{
			Address: cfg.Address, Username: cfg.Username, Password: cfg.Password,
			DialOptions: []grpc.DialOption{grpc.WithNoProxy()},
		})
		return dialErr
	})
	if err != nil {
		return nil, fmt.Errorf("router: milvus connect: %w", err)
	}
	index := &MilvusIndex{cli: cli, collection: cfg.Collection, dim: cfg.Dim}
	if err := retryStartup(connectCtx, func() error { return index.ensure(connectCtx) }); err != nil {
		_ = cli.Close()
		return nil, err
	}
	return index, nil
}

// retryStartup 只重试 Milvus 启动期错误，其余错误立即返回。
func retryStartup(ctx context.Context, fn func() error) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		message := strings.ToLower(err.Error())
		if !strings.Contains(message, "not ready") && !strings.Contains(message, "service unavailable") {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}

func (m *MilvusIndex) Close() error {
	if m == nil || m.cli == nil {
		return nil
	}
	return m.cli.Close()
}

func (m *MilvusIndex) ensure(ctx context.Context) error {
	exists, err := m.cli.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("router: milvus has collection: %w", err)
	}
	if !exists {
		schema := &entity.Schema{
			CollectionName: m.collection,
			Description:    "review seed bank (authoritative state in xbh_review.review_seed)",
			Fields: []*entity.Field{
				{Name: "seed_id", DataType: entity.FieldTypeInt64, PrimaryKey: true},
				{Name: "market", DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": "8"}},
				{Name: "issue", DataType: entity.FieldTypeVarChar, TypeParams: map[string]string{"max_length": "64"}},
				{Name: "embedding", DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": strconv.Itoa(m.dim)}},
			},
		}
		if err := m.cli.CreateCollection(ctx, schema, 1); err != nil {
			return fmt.Errorf("router: milvus create collection: %w", err)
		}
		idx, err := entity.NewIndexFlat(entity.IP)
		if err != nil {
			return fmt.Errorf("router: milvus build index: %w", err)
		}
		if err := m.cli.CreateIndex(ctx, m.collection, "embedding", idx, false); err != nil {
			return fmt.Errorf("router: milvus create index: %w", err)
		}
	}
	if err := m.cli.LoadCollection(ctx, m.collection, false); err != nil {
		return fmt.Errorf("router: milvus load collection: %w", err)
	}
	return nil
}

// Insert 写入一个种子向量（先删后插，重复同步幂等）。
func (m *MilvusIndex) Insert(ctx context.Context, seedID int64, market, issue string, vector []float32) error {
	if len(vector) != m.dim {
		return fmt.Errorf("router: seed vector dim %d, want %d", len(vector), m.dim)
	}
	if err := m.Delete(ctx, []int64{seedID}); err != nil {
		return err
	}
	if _, err := m.cli.Insert(ctx, m.collection, "",
		entity.NewColumnInt64("seed_id", []int64{seedID}),
		entity.NewColumnVarChar("market", []string{market}),
		entity.NewColumnVarChar("issue", []string{issue}),
		entity.NewColumnFloatVector("embedding", m.dim, [][]float32{normalize(vector)}),
	); err != nil {
		return fmt.Errorf("router: milvus insert: %w", err)
	}
	return m.cli.Flush(ctx, m.collection, false)
}

// Delete 移除种子向量。
func (m *MilvusIndex) Delete(ctx context.Context, seedIDs []int64) error {
	if len(seedIDs) == 0 {
		return nil
	}
	if err := m.cli.DeleteByPks(ctx, m.collection, "", entity.NewColumnInt64("seed_id", seedIDs)); err != nil {
		return fmt.Errorf("router: milvus delete: %w", err)
	}
	return nil
}

// Search 在市场内检索 top-k 种子。
func (m *MilvusIndex) Search(ctx context.Context, vector []float32, market string, topK int) ([]Hit, error) {
	params, err := entity.NewIndexFlatSearchParam()
	if err != nil {
		return nil, err
	}
	results, err := m.cli.Search(ctx, m.collection, nil, `market == "`+sanitizeMarket(market)+`"`,
		[]string{"seed_id", "issue"}, []entity.Vector{entity.FloatVector(vector)}, "embedding", entity.IP, topK, params,
		client.WithSearchQueryConsistencyLevel(entity.ClStrong))
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for _, result := range results {
		if result.Err != nil {
			return nil, result.Err
		}
		ids, okID := result.Fields.GetColumn("seed_id").(*entity.ColumnInt64)
		issues, okIssue := result.Fields.GetColumn("issue").(*entity.ColumnVarChar)
		if !okID || !okIssue || ids.Len() != result.ResultCount || issues.Len() != result.ResultCount || len(result.Scores) != result.ResultCount {
			return nil, fmt.Errorf("router: malformed milvus result")
		}
		for i := range result.ResultCount {
			hits = append(hits, Hit{SeedID: ids.Data()[i], Issue: issues.Data()[i], Score: float64(result.Scores[i])})
		}
	}
	return hits, nil
}

// 市场代码只允许大写字母，避免把输入拼进过滤表达式。
func sanitizeMarket(market string) string {
	var b strings.Builder
	for _, r := range market {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
