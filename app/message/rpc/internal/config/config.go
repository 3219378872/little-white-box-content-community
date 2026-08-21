package config

import (
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	DataSource string
	UserRpc    zrpc.RpcClientConf
	MediaRpc   zrpc.RpcClientConf
}
