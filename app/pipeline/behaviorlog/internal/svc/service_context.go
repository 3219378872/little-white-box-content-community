package svc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"esx/app/pipeline/behaviorlog/internal/config"
	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"util"

	"github.com/zeromicro/go-zero/core/bloom"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type BehaviorStore interface {
	Insert(ctx context.Context, e event.BehaviorEvent) error
}

type EventDeduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

type ServiceContext struct {
	Config config.Config
	Store  BehaviorStore
	Dedup  EventDeduper
	db     *sql.DB
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := validateBehaviorLogConfig(c); err != nil {
		panic(err)
	}
	if err := initSnowflake(c); err != nil {
		panic(fmt.Errorf("behavior-log: init snowflake: %w", err))
	}

	db, err := sql.Open("clickhouse", c.ClickHouseDSN)
	if err != nil {
		panic(fmt.Sprintf("behavior-log: open clickhouse: %v", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("behavior-log: ping clickhouse: %v", err))
	}

	rds := redis.MustNewRedis(c.Redis)
	bloomStore := NewRedisBloomStore(rds)

	return &ServiceContext{
		Config: c,
		Store:  store.NewClickHouseStore(db),
		Dedup:  dedup.NewBloomDedup(bloomStore, c.BloomBits),
		db:     db,
	}
}

func (s *ServiceContext) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func validateBehaviorLogConfig(c config.Config) error {
	missing := make([]string, 0, 5)
	if c.ClickHouseDSN == "" {
		missing = append(missing, "ClickHouseDSN")
	}
	if c.MQ.NameServer == "" {
		missing = append(missing, "MQ.NameServer")
	}
	if c.MQ.GroupName == "" {
		missing = append(missing, "MQ.GroupName")
	}
	if c.Redis.Host == "" {
		missing = append(missing, "Redis.Host")
	}
	if c.BloomBits == 0 {
		missing = append(missing, "BloomBits")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing behavior-log config: %s", strings.Join(missing, ", "))
	}
	return nil
}

func initSnowflake(c config.Config) error {
	workerID := c.WorkerID
	if workerID == 0 {
		workerID = 1
	}
	datacenterID := c.DatacenterID
	if datacenterID == 0 {
		datacenterID = 1
	}
	return util.InitSnowflake(workerID, datacenterID)
}

type RedisBloomStore struct {
	rds *redis.Redis
}

func NewRedisBloomStore(rds *redis.Redis) *RedisBloomStore {
	return &RedisBloomStore{rds: rds}
}

func (s *RedisBloomStore) Exists(ctx context.Context, key string, bits uint, data []byte) (bool, error) {
	return bloom.New(s.rds, key, bits).ExistsCtx(ctx, data)
}

func (s *RedisBloomStore) Add(ctx context.Context, key string, bits uint, data []byte) error {
	return bloom.New(s.rds, key, bits).AddCtx(ctx, data)
}

func (s *RedisBloomStore) Expire(ctx context.Context, key string, seconds int) error {
	return s.rds.ExpireCtx(ctx, key, seconds)
}
