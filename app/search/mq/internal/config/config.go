package config

import "mqx"

type Config struct {
	MQ mqx.ConsumerConfig
	ES ESConfig
}

// ESConfig 是 Elasticsearch 客户端配置。
// Addresses 为空时使用 NoopIndexer，便于本地起步与测试。
type ESConfig struct {
	Addresses []string `json:",optional"`
	Index     string   `json:",default=xbh_posts"`
	Username  string   `json:",optional"`
	Password  string   `json:",optional"`
}
