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

// NoopIndexer is the default no-op implementation.
type NoopIndexer struct{}

func (n *NoopIndexer) Index(ctx context.Context, doc IndexDoc) error {
	return nil
}

func (n *NoopIndexer) Delete(ctx context.Context, docID string) error {
	return nil
}
