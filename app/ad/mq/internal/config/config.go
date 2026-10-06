package config

import (
	"esx/pkg/mqx"
	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"

	service "esx/pkg/lifecycle"
)

// AssetStorage 与 ad-rpc 相同：私有桶与公开媒体桶。
type AssetStorage struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	PrivateBucket string
	PublicBucket  string
	PublicBaseURL string
}

// Config 是广告异步处理配置；对账与索引重建按各自周期运行。
type Config struct {
	service.ServiceConf
	MQ                  mqx.ConsumerConfig
	DataSource          string
	Redis               redis.RedisConf
	ReviewRpc           rpcx.RpcClientConf
	InternalSecret      string
	Assets              AssetStorage
	ReconcileIntervalMs int `json:",default=60000"`
	RebuildIntervalMs   int `json:",default=300000"`
}
