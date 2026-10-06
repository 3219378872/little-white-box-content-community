package config

import (
	"esx/pkg/rpcx"
)

// Config 是消息 RPC 配置：数据库、用户服务与可选的媒体服务（发送媒体消息时校验归属）。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	UserRpc        rpcx.RpcClientConf
	MediaRpc       rpcx.RpcClientConf
}
