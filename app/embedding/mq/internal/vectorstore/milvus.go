package vectorstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// waitReady 轮询 HasCollection 直到 Milvus proxy 就绪或 ctx 超时。
// Milvus standalone 启动后 NewClient 立即返回，但 proxy 可能仍在初始化。
func (m *MilvusVectorStore) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(90 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for {
		_, err := m.cli.HasCollection(ctx, "__probe_collection__")
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "not ready") && !strings.Contains(err.Error(), "service unavailable") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("milvus not ready before deadline: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// MilvusVectorStore 把 PostEvent 产出的向量写入 Milvus collection。
// Schema 与父 spec §4.4 / §8 一致：post_id (Int64 PK) + embedding (FloatVector dim)。
type MilvusVectorStore struct {
	cli        client.Client
	collection string
	dim        int
}

// MilvusOption 配置 MilvusVectorStore。
type MilvusOption func(*milvusOptions)

type milvusOptions struct {
	username string
	password string
	dbName   string
}

func WithMilvusAuth(user, password string) MilvusOption {
	return func(o *milvusOptions) {
		o.username = user
		o.password = password
	}
}

func WithMilvusDatabase(db string) MilvusOption {
	return func(o *milvusOptions) {
		o.dbName = db
	}
}

func NewMilvusVectorStore(ctx context.Context, addr, collection string, dim int, opts ...MilvusOption) (*MilvusVectorStore, error) {
	o := &milvusOptions{}
	for _, opt := range opts {
		opt(o)
	}
	cfg := client.Config{
		Address:  addr,
		Username: o.username,
		Password: o.password,
		DBName:   o.dbName,
	}
	cli, err := client.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("milvus connect: %w", err)
	}
	return &MilvusVectorStore{cli: cli, collection: collection, dim: dim}, nil
}

// EnsureCollection 创建 collection 与 index（若不存在）。
// 首次调用前会等待 Milvus proxy 就绪，避免容器启动期 race。
func (m *MilvusVectorStore) EnsureCollection(ctx context.Context) error {
	if err := m.waitReady(ctx); err != nil {
		return err
	}
	exists, err := m.cli.HasCollection(ctx, m.collection)
	if err != nil {
		return fmt.Errorf("milvus has collection: %w", err)
	}
	if !exists {
		schema := &entity.Schema{
			CollectionName: m.collection,
			Description:    "post embeddings for search/recommend",
			AutoID:         false,
			Fields: []*entity.Field{
				{Name: "post_id", DataType: entity.FieldTypeInt64, PrimaryKey: true, AutoID: false},
				{Name: "embedding", DataType: entity.FieldTypeFloatVector, TypeParams: map[string]string{"dim": fmt.Sprintf("%d", m.dim)}},
			},
			EnableDynamicField: false,
		}
		if err := m.cli.CreateCollection(ctx, schema, 2); err != nil {
			return fmt.Errorf("milvus create collection: %w", err)
		}
		idx, err := entity.NewIndexIvfFlat(entity.L2, 128)
		if err != nil {
			return fmt.Errorf("milvus build index: %w", err)
		}
		if err := m.cli.CreateIndex(ctx, m.collection, "embedding", idx, false); err != nil {
			return fmt.Errorf("milvus create index: %w", err)
		}
	}
	if err := m.cli.LoadCollection(ctx, m.collection, false); err != nil {
		return fmt.Errorf("milvus load collection: %w", err)
	}
	return nil
}

func (m *MilvusVectorStore) Upsert(ctx context.Context, postID int64, vec []float32) error {
	if len(vec) != m.dim {
		return fmt.Errorf("vector dim mismatch: got %d, want %d", len(vec), m.dim)
	}
	idCol := entity.NewColumnInt64("post_id", []int64{postID})
	vecCol := entity.NewColumnFloatVector("embedding", m.dim, [][]float32{vec})
	if _, err := m.cli.Upsert(ctx, m.collection, "", idCol, vecCol); err != nil {
		return fmt.Errorf("milvus upsert: %w", err)
	}
	return nil
}

func (m *MilvusVectorStore) Delete(ctx context.Context, postID int64) error {
	expr := fmt.Sprintf("post_id == %d", postID)
	if err := m.cli.Delete(ctx, m.collection, "", expr); err != nil {
		return fmt.Errorf("milvus delete: %w", err)
	}
	return nil
}

func (m *MilvusVectorStore) Close() error {
	return m.cli.Close()
}
