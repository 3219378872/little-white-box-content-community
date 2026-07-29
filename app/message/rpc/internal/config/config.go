package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	UserRpc    zrpc.RpcClientConf
	MQ         mqx.ConsumerConfig
}
