package svc

import (
	"esx/app/media/mq/internal/config"
	"esx/app/media/mq/internal/storage"
	"fmt"
)

type ServiceContext struct {
	Config  config.Config
	Storage storage.ObjectStorage
}

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
