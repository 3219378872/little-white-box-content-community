package config

import "github.com/zeromicro/go-zero/zrpc"
import "jwtx"

type Config struct {
	zrpc.RpcServerConf
	JwtConfig jwtx.JwtConfig
}
