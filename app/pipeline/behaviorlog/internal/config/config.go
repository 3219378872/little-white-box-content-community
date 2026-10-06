package config

import (
	"esx/pkg/mqx"

	service "esx/pkg/lifecycle"
	redis "esx/pkg/redisstore"
)

// Config 是行为日志消费者配置；DedupTTL 是事件去重标记的保留秒数（默认 90 天）。
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
