package config

import (
	"esx/pkg/outboxx"
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	DataSource     string
	MediaRpc       zrpc.RpcClientConf
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
}
