package svc

import (
	"database/sql"
	"fmt"

	"esx/app/content/mq/cleanup/internal/config"
	"esx/app/content/mq/cleanup/internal/store"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config         config.Config
	CleanupStore   store.CleanupStore
	CountSyncStore store.CountSyncStore
	RawDB          *sql.DB
}

func NewServiceContext(c config.Config) *ServiceContext {
	rds := redis.MustNewRedis(c.Redis)
	var rawDB *sql.DB
	var countSync store.CountSyncStore
	if c.DataSource != "" {
		var err error
		rawDB, err = sql.Open("mysql", c.DataSource)
		if err != nil {
			panic(fmt.Sprintf("content cleanup database connection failed: %v", err))
		}
		countSync = store.NewCountSyncStore(rawDB, rds)
	}
	return &ServiceContext{
		Config:         c,
		CleanupStore:   store.NewRedisCleanupStore(rds),
		CountSyncStore: countSync,
		RawDB:          rawDB,
	}
}

func (s *ServiceContext) Close() error {
	if s == nil || s.RawDB == nil {
		return nil
	}
	return s.RawDB.Close()
}
