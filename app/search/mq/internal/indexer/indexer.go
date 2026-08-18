package indexer

import "context"

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
}
