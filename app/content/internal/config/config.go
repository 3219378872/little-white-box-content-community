package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	Redis      cache.NodeConf
	MQ         mqx.ProducerConfig
}
