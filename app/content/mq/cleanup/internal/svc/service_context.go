package svc

import (
	"esx/app/content/mq/cleanup/internal/config"
	"esx/app/content/mq/cleanup/internal/store"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config       config.Config
	CleanupStore store.CleanupStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	rds := redis.MustNewRedis(c.Redis)
	return &ServiceContext{
		Config:       c,
		CleanupStore: store.NewRedisCleanupStore(rds),
	}
}
