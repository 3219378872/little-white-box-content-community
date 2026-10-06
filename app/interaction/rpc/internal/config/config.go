package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	"esx/pkg/rpcx"
)

// Config 是互动 RPC 配置：数据库、MQ/outbox 与用于校验目标可互动的内容服务。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
	ContentRpc     rpcx.RpcClientConf `json:",optional"`
}
