package svc

import (
	"context"
	"fmt"
	"time"

	"esx/app/embedding/mq/internal/config"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"
)

type ServiceContext struct {
	Config      config.Config
	Embedder    embedder.Embedder
	VectorStore vectorstore.VectorStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:      c,
		Embedder:    embedder.NoopEmbedder{},
		VectorStore: buildVectorStore(c.Milvus),
	}
}

func buildVectorStore(cfg config.MilvusConfig) vectorstore.VectorStore {
	if cfg.Address == "" {
		return vectorstore.NoopVectorStore{}
	}
	opts := []vectorstore.MilvusOption{}
	if cfg.Username != "" {
		opts = append(opts, vectorstore.WithMilvusAuth(cfg.Username, cfg.Password))
	}
	if cfg.Database != "" {
		opts = append(opts, vectorstore.WithMilvusDatabase(cfg.Database))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := vectorstore.NewMilvusVectorStore(ctx, cfg.Address, cfg.Collection, cfg.Dim, opts...)
	if err != nil {
		fmt.Printf("embedding-mq: Milvus connect failed (%v), falling back to Noop\n", err)
		return vectorstore.NoopVectorStore{}
	}
	if err := store.EnsureCollection(ctx); err != nil {
		fmt.Printf("embedding-mq: EnsureCollection failed (%v), falling back to Noop\n", err)
		return vectorstore.NoopVectorStore{}
	}
	return store
}
