package svc

import (
	"esx/app/media/mq/internal/config"
	"esx/app/media/mq/internal/storage"
	"fmt"
)

// ServiceContext 持有清理消费者的配置与对象存储。
type ServiceContext struct {
	Config  config.Config
	Storage storage.ObjectStorage
}

// NewServiceContext 创建对象存储客户端，失败时终止启动。
func NewServiceContext(c config.Config) *ServiceContext {
	s3Client, err := storage.NewS3Client(c.S3Storage)
	if err != nil {
		panic(fmt.Sprintf("media-mq: s3 client init failed: %v", err))
	}
	return &ServiceContext{
		Config:  c,
		Storage: s3Client,
	}
}
