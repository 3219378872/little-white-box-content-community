package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	DataSource      string
	MQ              mqx.ConsumerConfig
	UserRpc         zrpc.RpcClientConf
	BigVThreshold   int64
	FanoutBatchSize int64
}
