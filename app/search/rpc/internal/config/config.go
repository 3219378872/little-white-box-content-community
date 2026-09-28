package config

import "esx/pkg/rpcx"

type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	ES             ESConfig
	UserRpc        rpcx.RpcClientConf
	ContentRpc     rpcx.RpcClientConf
}

type ESConfig struct {
	Addresses            []string
	Index                string `json:",default=xbh_posts"`
	Username             string `json:",optional"`
	Password             string `json:",optional"`
	StartupTimeoutMillis int64  `json:",default=10000"`
}
