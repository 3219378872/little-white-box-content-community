package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	Redis      redis.RedisConf
	MQ         mqx.ConsumerConfig
}
