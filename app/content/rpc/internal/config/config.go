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
	MediaRpc       rpcx.RpcClientConf
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
}
