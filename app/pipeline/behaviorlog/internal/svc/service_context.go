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

	redis "esx/pkg/redisstore"
)

// BehaviorStore 是行为事件的持久化与日聚合接口。
type BehaviorStore interface {
	Insert(ctx context.Context, behavior event.BehaviorEvent) error
	// AggregateDaily 把 [from,to) 窗口内的原始行为聚合进 daily_aggregates（REL-020）。
	AggregateDaily(ctx context.Context, from, to time.Time) (int64, error)
}

// EventDeduper 是事件去重接口。
type EventDeduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

// DeadLetterStore 是死信写入接口。
type DeadLetterStore interface {
	InsertDeadLetter(ctx context.Context, letter store.DeadLetter) error
}

// ServiceContext 持有行为日志消费者的存储、去重与死信依赖。
type ServiceContext struct {
	Config      config.Config
	Store       BehaviorStore
	Dedup       EventDeduper
	DeadLetters DeadLetterStore
	db          *sql.DB
}

// NewServiceContext 连接并探活 ClickHouse，装配 Redis 去重；任一依赖不可用时终止启动。
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

// Close 关闭 ClickHouse 连接。
func (s *ServiceContext) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// validateBehaviorLogConfig 一次列出所有缺失的配置项，便于部署时一次修正。
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

// RedisExactStore 用 Redis 键保存去重标记。
type RedisExactStore struct {
	rds *redis.Redis
}

// NewRedisExactStore 创建基于 Redis 的去重标记存储。
func NewRedisExactStore(rds *redis.Redis) *RedisExactStore {
	return &RedisExactStore{rds: rds}
}

// Exists 判断标记是否存在。
func (s *RedisExactStore) Exists(ctx context.Context, key string) (bool, error) {
	return s.rds.ExistsCtx(ctx, key)
}

// Set 写入带过期时间的标记。
func (s *RedisExactStore) Set(ctx context.Context, key, value string, seconds int) error {
	return s.rds.SetexCtx(ctx, key, value, seconds)
}
