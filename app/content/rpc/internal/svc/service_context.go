package svc

import (
	"context"
	"database/sql"
	"errors"
	"esx/app/content/rpc/internal/config"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/media/rpc/mediaservice"
	"esx/pkg/outboxx"
	"fmt"
	"interceptor"
	"mqx"
	"os"
	"strconv"
	"util"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config              config.Config
	DB                  *sql.DB
	Conn                sqlx.SqlConn
	PostModel           model2.PostModel
	CommentModel        model2.CommentModel
	CommentCommandModel model2.CommentCommandModel
	TagModel            model2.TagModel
	PostTagModel        model2.PostTagModel
	PostCommandModel    model2.PostCommandModel
	MediaService        mediaservice.MediaService
	OutboxStore         *outboxx.SQLStore
	OutboxRelay         *outboxx.Relay
	MQProducer          *mqx.Producer
}

func NewServiceContext(c config.Config) *ServiceContext {
	db, err := sql.Open("mysql", c.DataSource)
	if err != nil {
		panic(fmt.Sprintf("数据库连接失败: %v", err))
	}
	conn := sqlx.NewSqlConnFromDB(db)

	workerIdStr := os.Getenv("SNOWFLAKE_WORKER_ID")
	dataCenterIdStr := os.Getenv("SNOWFLAKE_DATACENTER_ID")
	var workerId, dataCenterId int64
	if workerIdStr != "" {
		id, parseErr := strconv.ParseInt(workerIdStr, 10, 64)
		if parseErr != nil {
			panic(fmt.Errorf("SNOWFLAKE_WORKER_ID 格式无效: %w", parseErr))
		}
		workerId = id
	}
	if dataCenterIdStr != "" {
		id, parseErr := strconv.ParseInt(dataCenterIdStr, 10, 64)
		if parseErr != nil {
			panic(fmt.Errorf("SNOWFLAKE_DATACENTER_ID 格式无效: %w", parseErr))
		}
		dataCenterId = id
	}
	if workerId == 0 {
		workerId = 1
	}
	if dataCenterId == 0 {
		dataCenterId = 1
	}
	if err = util.InitSnowflake(workerId, dataCenterId); err != nil {
		panic(fmt.Errorf("雪花算法初始化失败%v", err))
	}

	cacheConf := cache.CacheConf{
		cache.NodeConf{
			RedisConf: c.Redis.RedisConf,
			Weight:    100,
		},
	}

	var producer *mqx.Producer
	if c.MQ.NameServer != "" {
		producer, err = mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Errorf("RocketMQ producer 初始化失败: %w", err))
		}
	}
	outboxStore := outboxx.NewSQLStore(conn)
	var outboxRelay *outboxx.Relay
	if producer != nil {
		outboxRelay, err = outboxx.NewRelay(outboxStore, outboxx.PublisherFunc(func(ctx context.Context, record outboxx.Record) error {
			_, publishErr := producer.Send(ctx, mqx.Message{
				Topic: record.Topic, Tag: record.Tag, Key: record.Key, Body: record.Payload,
			})
			return publishErr
		}), c.Outbox.RelayConfig("content"))
		if err != nil {
			panic(fmt.Errorf("content outbox relay initialization failed: %w", err))
		}
	}
	postModel := model2.NewPostModel(conn, cacheConf)
	var mediaService mediaservice.MediaService
	if len(c.MediaRpc.Etcd.Hosts) > 0 || len(c.MediaRpc.Endpoints) > 0 || c.MediaRpc.Target != "" {
		mediaClient := zrpc.MustNewClient(c.MediaRpc,
			zrpc.WithUnaryClientInterceptor(interceptor.BizErrorUnaryInterceptor()),
			zrpc.WithUnaryClientInterceptor(interceptor.InternalAuthUnaryClientInterceptor(c.InternalSecret)))
		mediaService = mediaservice.NewMediaService(mediaClient)
	}

	return &ServiceContext{
		Config:              c,
		DB:                  db,
		Conn:                conn,
		PostModel:           postModel,
		CommentModel:        model2.NewCommentModel(conn, cacheConf),
		CommentCommandModel: model2.NewCommentCommandModel(conn, outboxStore),
		TagModel:            model2.NewTagModel(conn, cacheConf),
		PostTagModel:        model2.NewPostTagModel(conn, cacheConf),
		PostCommandModel:    model2.NewPostCommandModel(conn, outboxStore),
		MediaService:        mediaService,
		OutboxStore:         outboxStore,
		OutboxRelay:         outboxRelay,
		MQProducer:          producer,
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
