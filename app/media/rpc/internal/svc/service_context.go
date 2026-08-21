package svc

import (
	"context"
	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/storage"
	"esx/pkg/outboxx"
	"fmt"

	"esx/pkg/mqx"
	"esx/pkg/util"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config            config.Config
	Conn              sqlx.SqlConn
	MediaModel        model.MediaModel
	MediaCommandModel model.MediaCommandModel
	Storage           storage.ObjectStorage
	MQProducer        *mqx.Producer
	OutboxStore       *outboxx.SQLStore
	OutboxRelay       *outboxx.Relay
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := util.InitSnowflake(4, 1); err != nil {
		panic(fmt.Sprintf("media snowflake initialization failed: %v", err))
	}
	conn := sqlx.NewMysql(c.DataSource)

	cacheConf := cache.CacheConf{
		cache.NodeConf{
			RedisConf: c.Redis.RedisConf,
			Weight:    100,
		},
	}

	s3Client, err := storage.NewS3Client(c.S3Storage)
	if err != nil {
		panic(fmt.Sprintf("media: 对象存储初始化失败: %v", err))
	}

	var mqProducer *mqx.Producer
	if c.MQ.NameServer != "" {
		p, err := mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Sprintf("media: MQ producer initialization failed: %v", err))
		}
		mqProducer = p
	}

	outboxStore := outboxx.NewSQLStore(conn)
	var outboxRelay *outboxx.Relay
	if mqProducer != nil {
		outboxRelay, err = outboxx.NewRelay(outboxStore, outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, publishErr := mqProducer.Send(ctx, mqx.Message{
				Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload,
			})
			return publishErr
		}), c.Outbox.RelayConfig("media"))
		if err != nil {
			panic(fmt.Sprintf("media: outbox relay initialization failed: %v", err))
		}
	}

	return &ServiceContext{
		Config:            c,
		Conn:              conn,
		MediaModel:        model.NewMediaModel(conn, cacheConf),
		MediaCommandModel: model.NewMediaCommandModel(conn, outboxStore),
		Storage:           s3Client,
		MQProducer:        mqProducer,
		OutboxStore:       outboxStore,
		OutboxRelay:       outboxRelay,
	}
}

func (s *ServiceContext) RunOutboxRelay(ctx context.Context) error {
	if s == nil || s.OutboxRelay == nil {
		return nil
	}
	return s.OutboxRelay.Run(ctx)
}
