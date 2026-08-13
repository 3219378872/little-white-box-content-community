package svc

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"esx/app/pipeline/behaviorlog/internal/config"
	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type BehaviorStore interface {
	Insert(ctx context.Context, behavior event.BehaviorEvent) error
	// AggregateDaily 把 [from,to) 窗口内的原始行为聚合进 daily_aggregates（REL-020）。
	AggregateDaily(ctx context.Context, from, to time.Time) (int64, error)
}

type EventDeduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

type DeadLetterStore interface {
	InsertDeadLetter(ctx context.Context, letter store.DeadLetter) error
}

type ServiceContext struct {
	Config      config.Config
	Store       BehaviorStore
	Dedup       EventDeduper
	DeadLetters DeadLetterStore
	db          *sql.DB
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := validateBehaviorLogConfig(c); err != nil {
		panic(err)
	}
	db, err := sql.Open("clickhouse", c.ClickHouseDSN)
	if err != nil {
		panic(fmt.Sprintf("behavior-log: open clickhouse: %v", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("behavior-log: ping clickhouse: %v", err))
	}

	rds := redis.MustNewRedis(c.Redis)
	redisStore := NewRedisExactStore(rds)
	clickhouseStore := store.NewClickHouseStore(db)
	return &ServiceContext{
		Config: c, Store: clickhouseStore,
		Dedup:       dedup.NewExactDedup(redisStore, c.DedupTTL),
		DeadLetters: clickhouseStore, db: db,
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
	if c.DedupTTL <= 0 {
		missing = append(missing, "DedupTTL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing behavior-log config: %s", strings.Join(missing, ", "))
	}
	return nil
}

type RedisExactStore struct {
	rds *redis.Redis
}

func NewRedisExactStore(rds *redis.Redis) *RedisExactStore {
	return &RedisExactStore{rds: rds}
}

func (s *RedisExactStore) Exists(ctx context.Context, key string) (bool, error) {
	return s.rds.ExistsCtx(ctx, key)
}

func (s *RedisExactStore) Set(ctx context.Context, key, value string, seconds int) error {
	return s.rds.SetexCtx(ctx, key, value, seconds)
}
