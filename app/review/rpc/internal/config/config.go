package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/rpcx"
)

type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
	// 当前生效的审核政策版本（app/review/internal/policy/versions）
	PolicyVersion string
}
