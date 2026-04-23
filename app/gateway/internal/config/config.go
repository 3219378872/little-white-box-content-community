// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package config

import (
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
}
