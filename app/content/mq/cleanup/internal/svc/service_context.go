package svc

import (
	"database/sql"
	"fmt"

	"esx/app/content/mq/cleanup/internal/config"
	"esx/app/content/mq/cleanup/internal/store"

	redis "esx/pkg/redisstore"
)

// ServiceContext 持有清理与计数同步存储；未配置数据库时不启用计数同步。
type ServiceContext struct {
	Config         config.Config
	CleanupStore   store.CleanupStore
	CountSyncStore store.CountSyncStore
	RawDB          *sql.DB
}

// NewServiceContext 连接 Redis，配置了数据库时再创建计数同步存储。
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

// Close 关闭数据库连接。
func (s *ServiceContext) Close() error {
	if s == nil || s.RawDB == nil {
		return nil
	}
	return s.RawDB.Close()
}
