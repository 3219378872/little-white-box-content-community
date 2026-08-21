package config

import (
	"fmt"
	"strings"

	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	DataSource      string
	UserRpc         zrpc.RpcClientConf
	ContentRpc      zrpc.RpcClientConf
	RecommendRpc    zrpc.RpcClientConf
	CursorSecret    string
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
