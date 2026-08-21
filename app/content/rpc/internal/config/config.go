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
	MediaRpc       zrpc.RpcClientConf
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
}
