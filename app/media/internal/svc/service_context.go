package svc

import (
	"fmt"

	"esx/app/media/internal/config"
	"esx/app/media/internal/model"
	"esx/app/media/internal/storage"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config     config.Config
	Conn       sqlx.SqlConn
	MediaModel model.MediaModel
	Storage    storage.ObjectStorage
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

	return &ServiceContext{
		Config:     c,
		Conn:       conn,
		MediaModel: model.NewMediaModel(conn, cacheConf),
		Storage:    s3Client,
	}
}
