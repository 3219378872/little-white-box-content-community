package config

import (
	"esx/pkg/mqx"

	"esx/pkg/rpcx"
)

type Config struct {
	rpcx.RpcServerConf
	InternalSecret       string
	MQ                   mqx.ProducerConfig
	MaxBatchSize         int   `json:",default=100"`
	MaxPastAgeHours      int64 `json:",default=720"`
	MaxFutureSkewSeconds int64 `json:",default=300"`
}
