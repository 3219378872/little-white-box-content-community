package svc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/embedding/mq/internal/config"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/rpcx"
	"io"
)

// ServiceContext 持有向量消费者的向量服务、Milvus 存储与内容服务客户端。
type ServiceContext struct {
	Config        config.Config
	Embedder      embedder.Embedder
	VectorStore   vectorstore.VectorStore
	Content       contentservice.ContentService
	contentCloser io.Closer

	grpcEmbedder *embedder.GRPCEmbedder
	milvusStore  *vectorstore.MilvusVectorStore
}

// NewServiceContext 校验配置并在启动超时内连接向量服务与 Milvus；任一依赖不可用时返回错误。
func NewServiceContext(ctx context.Context, c config.Config) (*ServiceContext, error) {
	if err := c.ValidateRuntime(); err != nil {
		return nil, err
	}
	grpcEmbedder, err := embedder.NewGRPCEmbedder(ctx, embedder.ClientConfig{
		Address:              c.Embedding.Address,
		ExpectedModelVersion: c.Embedding.ModelVersion,
		ExpectedDimension:    c.Embedding.Dim,
		Timeout:              time.Duration(c.Embedding.TimeoutMs) * time.Millisecond,
		MaxTextBytes:         c.Embedding.MaxTextBytes,
		MaxBatchSize:         c.Embedding.MaxBatchSize,
		MaxBatchBytes:        c.Embedding.MaxBatchBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize embedding client: %w", err)
	}

	store, err := newMilvusStore(ctx, c.Milvus)
	if err != nil {
		_ = grpcEmbedder.Close()
		return nil, err
	}
	if err := store.OpenCollection(ctx); err != nil {
		_ = store.Close()
		_ = grpcEmbedder.Close()
		return nil, fmt.Errorf("open required Milvus collection: %w", err)
	}
	contentClient, err := rpcx.NewClient(c.ContentRpc, rpcx.WithInternalAuth(c.InternalSecret))
	if err != nil {
		_ = store.Close()
		_ = grpcEmbedder.Close()
		return nil, fmt.Errorf("initialize Content RPC client: %w", err)
	}
	return &ServiceContext{
		Config: c, Embedder: grpcEmbedder, VectorStore: store,
		grpcEmbedder: grpcEmbedder, milvusStore: store,
		Content: contentservice.NewContentService(contentClient), contentCloser: contentClient,
	}, nil
}

// newMilvusStore 按可选的认证与数据库配置连接 Milvus 集合。
func newMilvusStore(ctx context.Context, cfg config.MilvusConfig) (*vectorstore.MilvusVectorStore, error) {
	opts := make([]vectorstore.MilvusOption, 0, 2)
	if cfg.Username != "" {
		opts = append(opts, vectorstore.WithMilvusAuth(cfg.Username, cfg.Password))
	}
	if cfg.Database != "" {
		opts = append(opts, vectorstore.WithMilvusDatabase(cfg.Database))
	}
	store, err := vectorstore.NewMilvusVectorStore(ctx, cfg.Address, cfg.Collection, cfg.Dim, opts...)
	if err != nil {
		return nil, fmt.Errorf("initialize Milvus client: %w", err)
	}
	return store, nil
}

// Close 关闭内容服务、Milvus 与向量服务连接，汇总关闭错误。
func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.contentCloser != nil {
		errs = append(errs, s.contentCloser.Close())
	}
	if s.milvusStore != nil {
		errs = append(errs, s.milvusStore.Close())
	}
	if s.grpcEmbedder != nil {
		errs = append(errs, s.grpcEmbedder.Close())
	}
	return errors.Join(errs...)
}
