package config

import (
	"esx/pkg/outboxx"
	"jwtx"
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	JwtConfig  jwtx.JwtConfig
	DataSource string
	MQ         mqx.ProducerConfig
	Outbox     outboxx.Config
}
