package config

import (
	"esx/pkg/mqx"
	"esx/pkg/outboxx"

	service "esx/pkg/lifecycle"
)

// Config 是审核 worker 配置：MQ、outbox、政策版本、机审轮询周期与可选的排序/向量/Milvus 依赖。
type Config struct {
	service.ServiceConf
	MQ         mqx.ConsumerConfig
	Producer   mqx.ProducerConfig
	Outbox     outboxx.Config
	DataSource string
	// 生效政策与可选的影子政策（RVW-016）
	PolicyVersion       string
	ShadowPolicyVersion string `json:",optional"`
	PollIntervalMs      int    `json:",default=500"`
	Ranker              struct {
		Address string `json:",optional"`
	}
	Embedding struct {
		Address string `json:",optional"`
		Dim     int    `json:",default=384"`
	}
	Milvus struct {
		Address    string `json:",optional"`
		Username   string `json:",optional"`
		Password   string `json:",optional"`
		Collection string `json:",default=review_seed_bank_v1"`
	}
}
