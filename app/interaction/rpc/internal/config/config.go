package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	"esx/pkg/rpcx"
)

type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
	ContentRpc     rpcx.RpcClientConf `json:",optional"`
}
