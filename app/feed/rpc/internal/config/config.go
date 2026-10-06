package config

import (
	"fmt"
	"strings"

	"esx/pkg/mqx"

	"esx/pkg/rpcx"
)

// Config 是 Feed RPC 配置；CursorSecret 用于签名兜底流游标，FeatureVersion 决定负反馈键空间。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret  string
	DataSource      string
	UserRpc         rpcx.RpcClientConf
	ContentRpc      rpcx.RpcClientConf
	RecommendRpc    rpcx.RpcClientConf
	CursorSecret    string
	FeatureVersion  string `json:",default=v2"`
	MQ              mqx.ConsumerConfig
	BigVThreshold   int64
	FanoutBatchSize int64
}

// Validate 确认必需的密钥已配置，缺失时提示对应环境变量。
func (c Config) Validate() error {
	if strings.TrimSpace(c.CursorSecret) == "" {
		return fmt.Errorf("feed CursorSecret is required; set FEED_CURSOR_SECRET")
	}
	if strings.TrimSpace(c.InternalSecret) == "" {
		return fmt.Errorf("feed InternalSecret is required; set RPC_INTERNAL_SECRET")
	}
	return nil
}
