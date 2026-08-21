package config

import (
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type Config struct {
	service.ServiceConf
	MQ            mqx.ConsumerConfig
	ClickHouseDSN string
	Redis         redis.RedisConf
	DedupTTL      int `json:",default=7776000"`
	// AggregateIntervalSeconds：REL-020 定时聚合周期（秒），默认 86400（每天一次）；
	// 0 表示禁用定时聚合。
	AggregateIntervalSeconds int `json:",default=86400"`
	// AggregateBackfillDays：每次聚合覆盖的历史天数（含昨天），默认 1；首次上线可调大
	// （如 90）回填存量原始行为。
	AggregateBackfillDays int `json:",default=1"`
}
