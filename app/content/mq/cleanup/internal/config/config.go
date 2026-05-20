package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	MQ    mqx.ConsumerConfig
	Redis redis.RedisConf
}
