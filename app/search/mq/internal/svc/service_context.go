package svc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/indexer"
)

type ServiceContext struct {
	Config  config.Config
	Indexer indexer.Indexer
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	searchIndexer, err := buildIndexer(c.ES)
	if err != nil {
		return nil, err
	}
	return &ServiceContext{Config: c, Indexer: searchIndexer}, nil
}

func buildIndexer(cfg config.ESConfig) (indexer.Indexer, error) {
	if len(cfg.Addresses) == 0 || strings.TrimSpace(cfg.Addresses[0]) == "" {
		return nil, fmt.Errorf("search-mq: ES address is required")
	}
	if strings.TrimSpace(cfg.Index) == "" {
		return nil, fmt.Errorf("search-mq: ES index is required")
	}
	opts := []indexer.ESOption{}
	if cfg.Username != "" {
		opts = append(opts, indexer.WithBasicAuth(cfg.Username, cfg.Password))
	}
	es, err := indexer.NewESIndexer(cfg.Addresses, cfg.Index, opts...)
	if err != nil {
		return nil, fmt.Errorf("search-mq: initialize ES indexer: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := es.EnsureIndex(ctx); err != nil {
		return nil, fmt.Errorf("search-mq: ensure ES index: %w", err)
	}
	return es, nil
}
