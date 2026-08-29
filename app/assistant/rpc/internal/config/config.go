package config

import (
	"github.com/zeromicro/go-zero/zrpc"
)

type SafetyConfig struct {
	Enabled      bool `json:",default=true"`
	BlockedTerms []string
	MaxScanRunes int `json:",default=10000,range=[100:50000]"`
}

type Config struct {
	zrpc.RpcServerConf
	InternalSecret     string
	DataSource         string `json:",optional"`
	SearchRpc          zrpc.RpcClientConf
	ContentRpc         zrpc.RpcClientConf
	MediaRpc           zrpc.RpcClientConf
	RecommendRpc       zrpc.RpcClientConf
	InteractionRpc     zrpc.RpcClientConf
	UserRpc            zrpc.RpcClientConf
	MaxMessageRunes    int `json:",default=2000,range=[1:10000]"`
	QuotaWindowSeconds int `json:",default=60,range=[1:86400]"`
	QuotaRequests      int `json:",default=20,range=[1:10000]"`
	Safety             SafetyConfig
}
