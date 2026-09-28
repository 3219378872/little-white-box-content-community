package config

import (
	"esx/pkg/mqx"

	"esx/pkg/rpcx"
)

type Config struct {
	InternalSecret  string
	DataSource      string
	MQ              mqx.ConsumerConfig
	UserRpc         rpcx.RpcClientConf
	BigVThreshold   int64
	FanoutBatchSize int64
}
