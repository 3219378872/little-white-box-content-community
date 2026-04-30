package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	MQ            mqx.ConsumerConfig
	ClickHouseDSN string
	Redis         redis.RedisConf
	BloomBits     uint
	WorkerID      int64
	DatacenterID  int64
}
