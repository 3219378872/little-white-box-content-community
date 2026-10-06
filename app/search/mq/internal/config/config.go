package config

import (
	"esx/pkg/mqx"

	service "esx/pkg/lifecycle"
)

// Config 是搜索索引消费者的配置：服务基础项、MQ 订阅与 ES 连接。
type Config struct {
	service.ServiceConf
	MQ mqx.ConsumerConfig
	ES ESConfig
}

// ESConfig 是 Elasticsearch 客户端配置。在线消费者要求地址和索引均有效；
// 初始化失败时直接终止启动，避免确认消息却没有写入索引。
type ESConfig struct {
	Addresses []string `json:",optional"`
	Index     string   `json:",default=xbh_posts"`
	Username  string   `json:",optional"`
	Password  string   `json:",optional"`
}
