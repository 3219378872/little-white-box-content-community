package config

import (
	"fmt"
	"strings"

	"esx/pkg/mqx"

	"esx/pkg/rpcx"
)

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

func (c Config) Validate() error {
	if strings.TrimSpace(c.CursorSecret) == "" {
		return fmt.Errorf("feed CursorSecret is required; set FEED_CURSOR_SECRET")
	}
	if strings.TrimSpace(c.InternalSecret) == "" {
		return fmt.Errorf("feed InternalSecret is required; set RPC_INTERNAL_SECRET")
	}
	return nil
}
