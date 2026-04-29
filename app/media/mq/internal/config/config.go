package config

import (
	"esx/app/media/mq/internal/storage"
	"mqx"
)

type Config struct {
	S3Storage storage.Config
	MQ        mqx.ConsumerConfig
}
