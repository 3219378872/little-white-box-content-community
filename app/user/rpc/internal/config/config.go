package config

import (
	"jwtx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	JwtConfig  jwtx.JwtConfig
	DataSource string
}
