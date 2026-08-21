package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	DataSource     string
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
	ContentRpc     zrpc.RpcClientConf `json:",optional"`
}
