package config

import (
	"esx/pkg/mqx"

	"esx/pkg/rpcx"
)

// Config 是 fanout 消费者配置；粉丝数达到 BigVThreshold 的作者只写 outbox，不做写扩散。
type Config struct {
	InternalSecret  string
	DataSource      string
	MQ              mqx.ConsumerConfig
	UserRpc         rpcx.RpcClientConf
	BigVThreshold   int64
	FanoutBatchSize int64
}
