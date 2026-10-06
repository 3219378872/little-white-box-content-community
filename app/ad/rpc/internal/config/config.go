package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	"esx/pkg/rpcx"
)

// AssetStorage 是私有素材桶与公开媒体桶（ADS-015）。
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

// Config 是广告 RPC 配置：数据库、MQ/outbox、素材存储与投放缓存时长。
type Config struct {
	rpcx.RpcServerConf
	InternalSecret string
	DataSource     string
	MQ             mqx.ProducerConfig
	Outbox         outboxx.Config
	Assets         AssetStorage
	// 投放资格进程内缓存，不超过 30 秒（ADS-032）
	EligibilityCacheMs int `json:",default=10000"`
}
