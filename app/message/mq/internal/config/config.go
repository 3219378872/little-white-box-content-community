package config

import "mqx"

type Config struct {
	DataSource string
	Redis      struct {
		Host string
		Pass string
	}
	MQ mqx.ConsumerConfig
}
