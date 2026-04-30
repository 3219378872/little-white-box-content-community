package svc

import (
	"database/sql"
	"fmt"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"esx/app/pipeline/behaviorlog/internal/config"
	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config config.Config
	Store  store.BehaviorStore
	Dedup  *dedup.BloomDedup
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := sql.Open("clickhouse", c.ClickHouseDSN)
	if err != nil {
		panic(fmt.Sprintf("behavior-log: open clickhouse: %v", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("behavior-log: ping clickhouse: %v", err))
	}

	rds := redis.MustNewRedis(c.Redis)

	return &ServiceContext{
		Config: c,
		Store:  store.NewClickHouseStore(db),
		Dedup:  dedup.NewBloomDedup(rds, c.BloomBits),
	}
}
