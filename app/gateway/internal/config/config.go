// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package config

import (
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	RestConf rest.RestConf
	Auth     struct {
		AccessSecret string
		AccessExpire int64
	}
	UserRpc        zrpc.RpcClientConf
	ContentRpc     zrpc.RpcClientConf
	MediaRpc       zrpc.RpcClientConf
	InteractionRpc zrpc.RpcClientConf
	BehaviorRpc    zrpc.RpcClientConf
	FeedRpc        zrpc.RpcClientConf
	MessageRpc     zrpc.RpcClientConf
	SearchRpc      zrpc.RpcClientConf
	AssistantRpc   zrpc.RpcClientConf
}

// Validate 在启动前强制校验安全关键配置：空 JWT secret 会使 HS256
// 签名可被任意伪造，必须在进程启动时失败而不是静默运行。
func (c Config) Validate() error {
	if strings.TrimSpace(c.Auth.AccessSecret) == "" {
		return fmt.Errorf("gateway Auth.AccessSecret is required; set JWT_SECRET_KEY")
	}
	return nil
}
