package svc

import (
	"context"
	"database/sql"
	"esx/app/content/internal/config"
	"esx/app/content/internal/model"
	"fmt"
	"mqx"
	"os"
	"strconv"
	"util"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type MQProducer interface {
	SendSyncWithTag(ctx context.Context, topic, tag string, body []byte) (*primitive.SendResult, error)
}

type ServiceContext struct {
	Config               config.Config
	DB                   *sql.DB
	Conn                 sqlx.SqlConn
	PostModel            model.PostModel
	CommentModel         model.CommentModel
	TagModel             model.TagModel
	PostTagModel         model.PostTagModel
	MQProducer           MQProducer
	PostCreateMsgFactory PostCreateMsgFactory
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

	var producer MQProducer
	if c.MQ.NameServer != "" {
		producer, err = mqx.NewProducer(c.MQ)
		if err != nil {
			panic(fmt.Errorf("RocketMQ producer 初始化失败: %w", err))
		}
	}

	return &ServiceContext{
		Config:               c,
		DB:                   db,
		Conn:                 conn,
		PostModel:            model.NewPostModel(conn, cacheConf),
		CommentModel:         model.NewCommentModel(conn, cacheConf),
		TagModel:             model.NewTagModel(conn, cacheConf),
		PostTagModel:         model.NewPostTagModel(conn, cacheConf),
		MQProducer:           producer,
		PostCreateMsgFactory: DTMPostCreateMsgFactory{DtmServer: c.DtmServer, DB: db},
	}
}
