package config

import "esx/pkg/rpcx"

// Config 是搜索 RPC 的配置：ES 连接与用户/内容下游。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	ES             ESConfig
	UserRpc        rpcx.RpcClientConf
	ContentRpc     rpcx.RpcClientConf
}

// ESConfig 是 ES 读连接配置；StartupTimeoutMillis 限制启动时健康检查的等待时间。
type ESConfig struct {
	Addresses            []string
	Index                string `json:",default=xbh_posts"`
	Username             string `json:",optional"`
	Password             string `json:",optional"`
	StartupTimeoutMillis int64  `json:",default=10000"`
}
