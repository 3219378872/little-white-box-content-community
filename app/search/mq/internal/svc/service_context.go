package svc

import (
	"context"
	"fmt"
	"time"

	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/indexer"
)

type ServiceContext struct {
	Config  config.Config
	Indexer indexer.Indexer
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:  c,
		Indexer: buildIndexer(c.ES),
	}
}

func buildIndexer(cfg config.ESConfig) indexer.Indexer {
	if len(cfg.Addresses) == 0 {
		return &indexer.NoopIndexer{}
	}
	opts := []indexer.ESOption{}
	if cfg.Username != "" {
		opts = append(opts, indexer.WithBasicAuth(cfg.Username, cfg.Password))
	}
	es, err := indexer.NewESIndexer(cfg.Addresses, cfg.Index, opts...)
	if err != nil {
		return &indexer.NoopIndexer{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := es.EnsureIndex(ctx); err != nil {
		// 启动期索引不可创建：降级为 Noop，避免阻塞消费者起动；
		// 真实环境下 health check 会发现告警。
		fmt.Printf("search-mq: EnsureIndex failed (%v), falling back to Noop\n", err)
		return &indexer.NoopIndexer{}
	}
	return es
}
