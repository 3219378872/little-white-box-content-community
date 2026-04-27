package svc

import (
	"context"
	"database/sql"
	"esx/app/content/rpc/internal/config"
	model2 "esx/app/content/rpc/internal/model"
	"fmt"
	"mqx"
	"os"
	"strconv"
	"strings"
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
	PostModel            model2.PostModel
	CommentModel         model2.CommentModel
	TagModel             model2.TagModel
	PostTagModel         model2.PostTagModel
	MQProducer           MQProducer
	PostCreateMsgFactory PostCreateMsgFactory
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := validateDTMConfig(c); err != nil {
		panic(err)
	}
	configureDTMBarrierTable()

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
		PostModel:            model2.NewPostModel(conn, cacheConf),
		CommentModel:         model2.NewCommentModel(conn, cacheConf),
		TagModel:             model2.NewTagModel(conn, cacheConf),
		PostTagModel:         model2.NewPostTagModel(conn, cacheConf),
		MQProducer:           producer,
		PostCreateMsgFactory: DTMPostCreateMsgFactory{DtmServer: c.DtmServer, DB: db},
	}
}

func validateDTMConfig(c config.Config) error {
	missing := make([]string, 0, 3)
	if c.DtmServer == "" {
		missing = append(missing, "DtmServer")
	}
	if c.ContentBusiServer == "" {
		missing = append(missing, "ContentBusiServer")
	}
	if c.FeedBusiServer == "" {
		missing = append(missing, "FeedBusiServer")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing DTM content config: %s", strings.Join(missing, ", "))
	}
	return nil
}
