package config

import (
	"esx/pkg/rpcx"
)

// SafetyConfig 是记忆写入的屏蔽词过滤配置。
type SafetyConfig struct {
	Enabled      bool `json:",default=true"`
	BlockedTerms []string
	MaxScanRunes int `json:",default=10000,range=[100:50000]"`
}

// Config 是助手 RPC 服务的配置。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret     string
	DataSource         string `json:",optional"`
	SearchRpc          rpcx.RpcClientConf
	ContentRpc         rpcx.RpcClientConf
	MediaRpc           rpcx.RpcClientConf
	RecommendRpc       rpcx.RpcClientConf
	InteractionRpc     rpcx.RpcClientConf
	UserRpc            rpcx.RpcClientConf
	MaxMessageRunes    int `json:",default=2000,range=[1:10000]"`
	QuotaWindowSeconds int `json:",default=60,range=[1:86400]"`
	QuotaRequests      int `json:",default=20,range=[1:10000]"`
	Safety             SafetyConfig
}
