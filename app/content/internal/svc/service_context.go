package svc

import (
	"esx/app/content/internal/config"
	"esx/app/content/internal/model"
	"fmt"
	"os"
	"strconv"
	"util"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config       config.Config
	Conn         sqlx.SqlConn
	PostModel    model.PostModel
	CommentModel model.CommentModel
	TagModel     model.TagModel
	PostTagModel model.PostTagModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	// 连接MySQL
	conn, err := sqlx.NewConn(sqlx.SqlConf{
		DataSource: c.DataSource,
		DriverName: "mysql",
	})
	if err != nil {
		panic(fmt.Sprintf("数据库连接失败: %v", err))
	}

	// 从环境变量读取 WorkerId，默认 1；多实例部署时需为每个实例设置唯一值
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

	return &ServiceContext{
		Config:       c,
		Conn:         conn,
		PostModel:    model.NewPostModel(conn, cacheConf),
		CommentModel: model.NewCommentModel(conn, cacheConf),
		TagModel:     model.NewTagModel(conn, cacheConf),
		PostTagModel: model.NewPostTagModel(conn, cacheConf),
	}
}
