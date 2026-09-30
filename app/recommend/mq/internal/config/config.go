package config

import (
	"esx/pkg/mqx"
	"fmt"
	"strings"

	service "esx/pkg/lifecycle"
	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"
)

type Config struct {
	service.ServiceConf
	InternalSecret string
	UserRpc        rpcx.RpcClientConf
	MQ             mqx.ConsumerConfig
	Redis          redis.RedisConf
	FeatureVersion string `json:",default=v2"`
	FeatureTTL     int    `json:",default=2592000"`
	DedupTTL       int    `json:",default=7776000"`
	// The prior deployed FeatureTTL, used once to preserve original receipt age.
	LegacyDedupTTL               int    `json:",default=2592000"`
	DedupMigrationTimeoutSeconds int    `json:",default=30"`
	RecallKeyPrefix              string `json:",default=recommend"`
	CandidateTTL                 int    `json:",default=2592000"`
	DeadLetterTTL                int    `json:",default=604800"`
	DeadLetterMaxLength          int    `json:",default=1000"`
	// OptOutCleanupInterval 秒：REL-023 主动清理已关闭个性化用户在线特征的周期。
	// 默认 3600（1 小时），0 表示禁用定时清理。
	OptOutCleanupInterval int `json:",default=3600"`
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.InternalSecret) == "" {
		return fmt.Errorf("recommend consumer InternalSecret is required; set RPC_INTERNAL_SECRET")
	}
	// UniversalClient.Scan is node-local for Redis Cluster. Until migration
	// explicitly walks every master, a successful scan cannot prove that all
	// legacy receipts were upgraded. Reject this deployment before any clients,
	// migration-ready logs or consumers are started.
	if c.Redis.Type == redis.ClusterType {
		return fmt.Errorf("recommend consumer requires Redis Type=node: cluster-wide dedup retention migration is not supported")
	}
	if c.FeatureTTL <= 0 || c.FeatureTTL > 30*24*3600 {
		return fmt.Errorf("recommend consumer FeatureTTL must be within 30 days")
	}
	if c.DedupTTL != 90*24*3600 || c.LegacyDedupTTL <= 0 || c.LegacyDedupTTL > c.DedupTTL || c.DedupMigrationTimeoutSeconds <= 0 {
		return fmt.Errorf("recommend consumer dedup retention must be 90 days with a valid legacy TTL and migration timeout")
	}
	return nil
}
