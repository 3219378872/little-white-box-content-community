package config

import (
	"esx/pkg/outboxx"
	"mqx"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource        string
	DtmServer         string
	ContentBusiServer string
	FeedBusiServer    string
	MQ                mqx.ProducerConfig
	Outbox            outboxx.Config
}
