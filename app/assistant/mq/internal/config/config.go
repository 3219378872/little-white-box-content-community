package config

import (
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/service"
)

type Config struct {
	service.ServiceConf
	MQ         mqx.ConsumerConfig
	DataSource string
}
