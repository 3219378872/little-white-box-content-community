package config

import (
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	service.ServiceConf
	MQ               mqx.ConsumerConfig
	ContentRpc       zrpc.RpcClientConf
	InternalSecret   string
	DataSource       string
	SpikeMinComments int `json:",default=5"`
}
