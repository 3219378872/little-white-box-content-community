package svc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/config"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	redis "esx/pkg/redisstore"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"
)

// ServiceContext 持有广告 RPC 的存储、素材、投放索引与短期缓存；Clock 可在测试中替换。
type ServiceContext struct {
	Config      config.Config
	DB          *sql.DB
	Store       Store
	Assets      AssetStorage
	Redis       serving.KV
	Index       serving.Index
	Cache       *Cache
	OutboxRelay *outboxx.Relay
	MQProducer  *mqx.Producer
	Clock       func() time.Time
}

// NewServiceContext 装配数据库、ID 生成器、素材存储与 Redis；缓存 TTL 限制在 1～30 秒（ADS-032）。
func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := sqlx.NewConn(sqlx.SqlConf{DataSource: c.DataSource, DriverName: "mysql"})
	if err != nil {
		panic(fmt.Sprintf("ad rpc database initialization failed: %v", err))
	}
	rawDB, err := conn.RawDB()
	if err != nil {
		panic(fmt.Sprintf("ad rpc database access failed: %v", err))
	}
	if err := util.InitSnowflakeFromEnv(12, 1); err != nil {
		panic(fmt.Sprintf("ad rpc snowflake initialization failed: %v", err))
	}
	storage, err := assets.New(context.Background(), assets.Config{
		Endpoint: c.Assets.Endpoint, AccessKey: c.Assets.AccessKey, SecretKey: c.Assets.SecretKey,
		UseSSL: c.Assets.UseSSL, Region: c.Assets.Region, PrivateBucket: c.Assets.PrivateBucket,
		PublicBucket: c.Assets.PublicBucket, PublicBaseURL: c.Assets.PublicBaseURL,
	})
	if err != nil {
		panic(fmt.Sprintf("ad rpc asset storage initialization failed: %v", err))
	}
	kv := redis.MustNewRedis(c.Redis.RedisConf)
	adStore := store.New(conn)
	s := &ServiceContext{
		Config: c, DB: rawDB, Store: adStore, Assets: storage, Redis: kv, Index: serving.Index{KV: kv},
		Cache: NewCache(time.Duration(min(max(c.EligibilityCacheMs, 1000), 30000)) * time.Millisecond),
		Clock: time.Now,
	}
	// 未配置 MQ 时不创建 relay，送审事件留在 outbox 表中待配置后投递。
	if c.MQ.NameServer != "" {
		producer, err := mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Errorf("ad rpc RocketMQ producer initialization failed: %w", err))
		}
		relay, err := outboxx.NewRelay(adStore.Outbox(), outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, sendErr := producer.Send(ctx, mqx.Message{Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload})
			return sendErr
		}), c.Outbox.RelayConfig("ad-rpc"))
		if err != nil {
			panic(fmt.Errorf("ad rpc outbox relay initialization failed: %w", err))
		}
		s.MQProducer, s.OutboxRelay = producer, relay
	}
	return s
}

// Now 返回当前时间，优先使用注入的时钟。
func (s *ServiceContext) Now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// Close 关闭 MQ 生产者与数据库连接，汇总所有关闭错误。
func (s *ServiceContext) Close() error {
	var errs []error
	if s.MQProducer != nil {
		errs = append(errs, s.MQProducer.Shutdown())
	}
	if s.DB != nil {
		errs = append(errs, s.DB.Close())
	}
	return errors.Join(errs...)
}
