package config

import (
	"esx/pkg/mqx"

	service "esx/pkg/lifecycle"
	"esx/pkg/rpcx"
)

type Config struct {
	service.ServiceConf
	MQ               mqx.ConsumerConfig
	ContentRpc       rpcx.RpcClientConf
	InternalSecret   string
	DataSource       string
	SpikeMinComments int `json:",default=5"`
}
