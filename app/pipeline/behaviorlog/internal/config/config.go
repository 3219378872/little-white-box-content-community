package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	MQ            mqx.ConsumerConfig
	ClickHouseDSN string
	Redis         redis.RedisConf
	BloomBits     uint  `json:",default=20971520"`
	WorkerID      int64 `json:",default=1"`
	DatacenterID  int64 `json:",default=1"`
}
