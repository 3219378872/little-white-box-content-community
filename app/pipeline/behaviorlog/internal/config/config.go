package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	service.ServiceConf
	MQ            mqx.ConsumerConfig
	ClickHouseDSN string
	Redis         redis.RedisConf
	DedupTTL      int `json:",default=7776000"`
}
