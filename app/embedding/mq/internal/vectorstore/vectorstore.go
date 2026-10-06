package vectorstore

import "context"

// Record 是一条帖子向量；Revision 用于拒绝旧事件覆盖新数据。
type Record struct {
	PostID       int64
	Vector       []float32
	ModelVersion string
	Dimension    int
	Revision     int64
}

// VectorStore 是在线消费者对向量存储的读写。
type VectorStore interface {
	Upsert(ctx context.Context, record Record) error
	Delete(ctx context.Context, postID, revision int64) error
	CurrentRevision(ctx context.Context, postID int64) (int64, error)
}

// RebuildTarget 是离线重建写入新集合所需的操作。
type RebuildTarget interface {
	UpsertBatch(ctx context.Context, records []Record) error
	Flush(ctx context.Context) error
	Count(ctx context.Context) (int64, error)
	PromoteAlias(ctx context.Context, alias string) error
}
