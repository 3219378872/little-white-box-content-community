package config

import (
	"esx/pkg/outboxx"
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	DataSource string
	MQ         mqx.ProducerConfig
	Outbox     outboxx.Config
	ContentRpc zrpc.RpcClientConf `json:",optional"`
}
