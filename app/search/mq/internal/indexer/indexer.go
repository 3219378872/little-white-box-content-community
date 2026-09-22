package indexer

import (
	"context"
	"errors"
)

// ErrNotIndexed means a count patch arrived before the post document existed.
var ErrNotIndexed = errors.New("search document is not indexed")

// IndexDoc is a generic document for indexing.
type IndexDoc struct {
	DocID    string
	Type     string
	Revision int64
	Body     map[string]any
}

// Indexer is the future ES/Milvus write interface.
type Indexer interface {
	Index(ctx context.Context, doc IndexDoc) error
	Delete(ctx context.Context, docID string, revision int64) error
	PatchCounts(ctx context.Context, doc IndexDoc) error
}
