package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	"esx/pkg/rpcx"
)

// Config 是内容 RPC 配置：数据库、媒体服务（校验帖子图片归属）与 MQ/outbox。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	MediaRpc       rpcx.RpcClientConf
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
}
