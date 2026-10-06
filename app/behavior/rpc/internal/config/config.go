package config

import (
	"esx/pkg/mqx"

	"esx/pkg/rpcx"
)

// Config 是行为上报配置：单批事件上限，以及可接受的事件时间窗口（过去多少小时、未来多少秒）。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret       string
	MQ                   mqx.ProducerConfig
	MaxBatchSize         int   `json:",default=100"`
	MaxPastAgeHours      int64 `json:",default=720"`
	MaxFutureSkewSeconds int64 `json:",default=300"`
}
