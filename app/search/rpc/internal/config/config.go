package config

import "github.com/zeromicro/go-zero/zrpc"

type Config struct {
	zrpc.RpcServerConf
	ES      ESConfig
	UserRpc zrpc.RpcClientConf
}

type ESConfig struct {
	Addresses            []string
	Index                string `json:",default=xbh_posts"`
	Username             string `json:",optional"`
	Password             string `json:",optional"`
	StartupTimeoutMillis int64  `json:",default=10000"`
}
