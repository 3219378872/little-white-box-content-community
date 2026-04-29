package svc

import (
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"
)

type ServiceContext struct {
	Config        config.Config
	BehaviorStore store.BehaviorStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:        c,
		BehaviorStore: &store.NoopBehaviorStore{},
	}
}
