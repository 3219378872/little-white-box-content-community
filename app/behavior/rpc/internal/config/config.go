package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InternalSecret string
	MQ                   mqx.ProducerConfig
	MaxBatchSize         int   `json:",default=100"`
	MaxPastAgeHours      int64 `json:",default=720"`
	MaxFutureSkewSeconds int64 `json:",default=300"`
}
