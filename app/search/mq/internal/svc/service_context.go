package svc

import (
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
		Indexer: &indexer.NoopIndexer{},
	}
}
