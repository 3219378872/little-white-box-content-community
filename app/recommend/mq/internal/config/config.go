package config

import (
	"esx/pkg/mqx"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	service.ServiceConf
	InternalSecret      string
	UserRpc             zrpc.RpcClientConf
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

func (c Config) Validate() error {
	if strings.TrimSpace(c.InternalSecret) == "" {
		return fmt.Errorf("recommend consumer InternalSecret is required; set RPC_INTERNAL_SECRET")
	}
	return nil
}
