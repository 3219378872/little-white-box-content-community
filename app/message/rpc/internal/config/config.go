package config

import (
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	UserRpc    zrpc.RpcClientConf
	MediaRpc   zrpc.RpcClientConf
}
