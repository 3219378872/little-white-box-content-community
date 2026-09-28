package config

import (
	"fmt"
	"strings"

	"esx/pkg/lifecycle"
	"esx/pkg/rpcx"
)

type Config struct {
	RestConf HTTPConfig
	Auth     struct {
		AccessSecret string
		AccessExpire int64
	}
	UserRpc        rpcx.RpcClientConf
	ContentRpc     rpcx.RpcClientConf
	MediaRpc       rpcx.RpcClientConf
	InteractionRpc rpcx.RpcClientConf
	BehaviorRpc    rpcx.RpcClientConf
	FeedRpc        rpcx.RpcClientConf
	MessageRpc     rpcx.RpcClientConf
	SearchRpc      rpcx.RpcClientConf
	AssistantRpc   rpcx.RpcClientConf
	InternalSecret string
}

// Validate 在启动前强制校验安全关键配置：空 JWT secret 会使 HS256
// 签名可被任意伪造，必须在进程启动时失败而不是静默运行。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Auth.AccessSecret) == "" {
		return fmt.Errorf("gateway Auth.AccessSecret is required; set JWT_SECRET_KEY")
	}
	if strings.TrimSpace(c.InternalSecret) == "" {
		return fmt.Errorf("gateway InternalSecret is required; set RPC_INTERNAL_SECRET")
	}
	return nil
}

// HTTPConfig keeps the established deployment YAML stable while Hertz owns HTTP.
type HTTPConfig struct {
	lifecycle.ServiceConf
	Host     string
	Port     int
	Timeout  int64 `json:",default=3000"`
	MaxBytes int64 `json:",default=10485760"`
}
