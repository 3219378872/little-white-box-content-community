package config

import (
	"esx/app/media/mq/internal/storage"
	"esx/pkg/mqx"
)

// Config 是媒体清理消费者配置：对象存储与 MQ 订阅。
type Config struct {
	S3Storage storage.Config
	MQ        mqx.ConsumerConfig
}
