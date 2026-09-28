package config

import (
	"esx/pkg/rpcx"
)

type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	UserRpc        rpcx.RpcClientConf
	MediaRpc       rpcx.RpcClientConf
}
