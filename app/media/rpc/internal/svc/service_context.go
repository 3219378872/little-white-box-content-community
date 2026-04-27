package svc

import (
	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/model"
	"esx/app/media/rpc/internal/storage"
	"fmt"

	"mqx"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config     config.Config
	Conn       sqlx.SqlConn
	MediaModel model.MediaModel
	Storage    storage.ObjectStorage
	MQProducer *mqx.Producer
}

func NewServiceContext(c config.Config) *ServiceContext {
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

	return &ServiceContext{
		Config:     c,
		Conn:       conn,
		MediaModel: model.NewMediaModel(conn, cacheConf),
		Storage:    s3Client,
		MQProducer: mqProducer,
	}
}
