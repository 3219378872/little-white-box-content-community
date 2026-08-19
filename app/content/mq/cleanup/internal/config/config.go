package config

import (
	"fmt"
	"strings"

	"mqx"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	MQ         mqx.ConsumerConfig
	CountSync  mqx.ConsumerConfig `json:",optional"`
	Redis      redis.RedisConf
	DataSource string
}

// CountSyncConsumerConfig is the RocketMQ client for like/favorite count
// projection. It must use a different consumer group than cleanup: one
// process cannot Start two PushConsumers on the same group.
func (c Config) CountSyncConsumerConfig() (mqx.ConsumerConfig, error) {
	cfg := c.CountSync
	if strings.TrimSpace(cfg.NameServer) == "" {
		cfg.NameServer = c.MQ.NameServer
	}
	if cfg.MaxReconsumeTimes <= 0 {
		cfg.MaxReconsumeTimes = c.MQ.MaxReconsumeTimes
	}
	if strings.TrimSpace(cfg.GroupName) == "" {
		return mqx.ConsumerConfig{}, fmt.Errorf("count-sync-consumer: CountSync.GroupName is required")
	}
	if cfg.GroupName == c.MQ.GroupName {
		return mqx.ConsumerConfig{}, fmt.Errorf("count-sync-consumer: CountSync.GroupName must differ from MQ.GroupName")
	}
	return cfg, nil
}
