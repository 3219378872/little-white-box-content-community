package config

import (
	"mqx"

	"github.com/zeromicro/go-zero/core/service"
)

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
