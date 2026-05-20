package vectorstore

import "context"

// VectorStore 是 Milvus / Faiss 等向量库的抽象，
// 让消费者层不必直接依赖具体 SDK，便于单测 stub。
type VectorStore interface {
	Upsert(ctx context.Context, postID int64, vec []float32) error
	Delete(ctx context.Context, postID int64) error
}

// NoopVectorStore 在未配置 Milvus 时使用，写入直接成功。
type NoopVectorStore struct{}

func (NoopVectorStore) Upsert(_ context.Context, _ int64, _ []float32) error { return nil }
func (NoopVectorStore) Delete(_ context.Context, _ int64) error              { return nil }
