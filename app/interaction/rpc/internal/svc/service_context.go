package svc

import (
	"context"
	"database/sql"
	"errors"
	"esx/app/content/rpc/contentservice"
	"esx/app/interaction/rpc/internal/config"
	model2 "esx/app/interaction/rpc/internal/model"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/util"
	"fmt"

	"github.com/cloudwego/kitex/client/callopt"

	cache "esx/pkg/modelcache"
	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
	sqlx "esx/pkg/sqlstore"

	"golang.org/x/sync/singleflight"
)

// ServiceContext holds interaction storage, the outbox relay, the count cache
// and the content client used to check that targets are interactable.
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
	ContentService      ContentService
}

// ContentService is the subset Interaction uses to enforce CORE-034.
type ContentService interface {
	AssertInteractable(ctx context.Context, in *contentservice.AssertInteractableReq, opts ...callopt.Option) (*contentservice.AssertInteractableResp, error)
}

// NewServiceContext wires MySQL models, Redis, the outbox relay and the optional
// content client.
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
	if err := util.InitSnowflakeFromEnv(3, 1); err != nil {
		panic(fmt.Sprintf("interaction snowflake initialization failed: %v", err))
	}

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

	var contentService ContentService
	if len(c.ContentRpc.Etcd.Hosts) > 0 || len(c.ContentRpc.Endpoints) > 0 || c.ContentRpc.Target != "" {
		contentClient := rpcx.MustNewClient(c.ContentRpc,

			rpcx.WithInternalAuth(c.InternalSecret))
		contentService = contentservice.NewContentService(contentClient)
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
		ContentService:      contentService,
	}
}

// Close shuts down the MQ producer and the database pool, joining their errors.
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
