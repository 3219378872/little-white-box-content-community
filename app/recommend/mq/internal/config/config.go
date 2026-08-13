package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	service.ServiceConf
	MQ                  mqx.ConsumerConfig
	Redis               redis.RedisConf
	FeatureVersion      string `json:",default=v2"`
	FeatureTTL          int    `json:",default=2592000"`
	RecallKeyPrefix     string `json:",default=recommend"`
	CandidateTTL        int    `json:",default=2592000"`
	DeadLetterTTL       int    `json:",default=604800"`
	DeadLetterMaxLength int    `json:",default=1000"`
	// OptOutCleanupInterval 秒：REL-023 主动清理已关闭个性化用户在线特征的周期。
	// 默认 3600（1 小时），0 表示禁用定时清理。
	OptOutCleanupInterval int `json:",default=3600"`
}
