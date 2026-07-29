package indexer

import "context"

// IndexDoc is a generic document for indexing.
type IndexDoc struct {
	DocID string
	Type  string
	Body  map[string]any
}

// Indexer is the future ES/Milvus write interface.
type Indexer interface {
	Index(ctx context.Context, doc IndexDoc) error
	Delete(ctx context.Context, docID string) error
}
