package config

import (
	"fmt"
	"strings"

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

// Validate 在启动前强制校验安全关键配置：user rpc 负责签发 JWT，
// 空 AccessSecret 会使签名可被任意伪造，必须启动即失败。
func (c Config) Validate() error {
	if strings.TrimSpace(c.JwtConfig.AccessSecret) == "" {
		return fmt.Errorf("user JwtConfig.AccessSecret is required; set JWT_SECRET_KEY")
	}
	return nil
}
