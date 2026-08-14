package svc

import (
	"context"
	"database/sql"
	"errors"
	"esx/app/interaction/rpc/internal/config"
	model2 "esx/app/interaction/rpc/internal/model"
	"esx/pkg/outboxx"
	"fmt"
	"mqx"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"golang.org/x/sync/singleflight"
)

type ServiceContext struct {
	Config              config.Config
	Conn                sqlx.SqlConn
	DB                  *sql.DB
	FavoriteModel       model2.FavoriteModel
	LikeRecordModel     model2.LikeRecordModel
	ActionCountModel    model2.ActionCountModel
	InteractionCommands model2.InteractionCommandModel
	OutboxStore         *outboxx.SQLStore
	OutboxRelay         *outboxx.Relay
	MQProducer          *mqx.Producer
	Redis               *redis.Redis
	RedisStore          RedisStore
	SingleFlight        singleflight.Group
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := sqlx.NewConn(sqlx.SqlConf{
		DataSource: c.DataSource,
		DriverName: "mysql",
	})
	if err != nil {
		panic(fmt.Sprintf("数据库连接初始化错误%v", err))
	}
	rawDB, err := conn.RawDB()
	if err != nil {
		panic(fmt.Sprintf("数据库连接访问错误%v", err))
	}

	conf := cache.CacheConf{
		cache.NodeConf{
			RedisConf: c.Redis.RedisConf,
			Weight:    100,
		},
	}

	redisClient := redis.MustNewRedis(c.Redis.RedisConf)

	outboxStore := outboxx.NewSQLStore(conn)
	var producer *mqx.Producer
	var outboxRelay *outboxx.Relay
	if c.MQ.NameServer != "" {
		producer, err = mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Errorf("interaction RocketMQ producer initialization failed: %w", err))
		}
		outboxRelay, err = outboxx.NewRelay(outboxStore, outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, publishErr := producer.Send(ctx, mqx.Message{
				Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload,
			})
			return publishErr
		}), c.Outbox.RelayConfig("interaction"))
		if err != nil {
			panic(fmt.Errorf("interaction outbox relay initialization failed: %w", err))
		}
	}

	return &ServiceContext{
		Config:              c,
		Conn:                conn,
		DB:                  rawDB,
		FavoriteModel:       model2.NewFavoriteModel(conn, conf),
		LikeRecordModel:     model2.NewLikeRecordModel(conn, conf),
		ActionCountModel:    model2.NewActionCountModel(conn),
		InteractionCommands: model2.NewInteractionCommandModel(conn, outboxStore),
		OutboxStore:         outboxStore,
		OutboxRelay:         outboxRelay,
		MQProducer:          producer,
		Redis:               redisClient,
		RedisStore:          NewRedisStore(redisClient),
	}
}

func (s *ServiceContext) RunOutboxRelay(ctx context.Context) error {
	if s == nil || s.OutboxRelay == nil {
		return nil
	}
	return s.OutboxRelay.Run(ctx)
}

func (s *ServiceContext) Close() error {
	if s == nil {
		return nil
	}
	var closeErrors []error
	if s.MQProducer != nil {
		closeErrors = append(closeErrors, s.MQProducer.Shutdown())
	}
	if s.DB != nil {
		closeErrors = append(closeErrors, s.DB.Close())
	}
	return errors.Join(closeErrors...)
}
