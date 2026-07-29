package vectorstore

import "context"

type Record struct {
	PostID       int64
	Vector       []float32
	ModelVersion string
	Dimension    int
}

type VectorStore interface {
	Upsert(ctx context.Context, record Record) error
	Delete(ctx context.Context, postID int64) error
}

type RebuildTarget interface {
	UpsertBatch(ctx context.Context, records []Record) error
	Flush(ctx context.Context) error
	Count(ctx context.Context) (int64, error)
	PromoteAlias(ctx context.Context, alias string) error
}
