package config

import (
	service "esx/pkg/lifecycle"
	"esx/pkg/rpcx"
)

// LLMConfig 是主模型路由及其重试、计费与备用路由配置。
type LLMConfig struct {
	Enabled                        bool
	RouteID                        string `json:",default=primary"`
	Boundary                       string `json:",optional"`
	WireAPI                        string `json:",default=chat_completions"`
	Endpoint                       string
	APIKey                         string `json:",optional"`
	Model                          string
	TimeoutMs                      int64            `json:",default=90000,range=[1000:600000]"`
	MaxOutputTokens                int              `json:",default=32768,range=[1:65536]"`
	ContextWindowTokens            int              `json:",default=128000,range=[1000:2000000]"`
	PromptCostPerMillionTokens     float64          `json:",default=0"`
	CompletionCostPerMillionTokens float64          `json:",default=0"`
	CacheReadCostPerMillionTokens  float64          `json:",default=0"`
	CacheWriteCostPerMillionTokens float64          `json:",default=0"`
	ReasoningCostPerMillionTokens  float64          `json:",default=0"`
	AuxModel                       string           `json:",optional"`
	CanaryEnabled                  bool             `json:",default=true"`
	RetryMaxAttempts               int              `json:",default=3,range=[1:3]"`
	RetryBaseDelayMs               int64            `json:",default=250,range=[1:30000]"`
	RetryMaxDelayMs                int64            `json:",default=5000,range=[1:60000]"`
	RetryAfterMaxMs                int64            `json:",default=30000,range=[1:120000]"`
	Fallbacks                      []LLMRouteConfig `json:",optional"`
}

// LLMRouteConfig 是一条备用路由；窗口、输出上限与数据边界须与主路由兼容才会被启用。
type LLMRouteConfig struct {
	Enabled                        bool `json:",default=true"`
	RouteID                        string
	Boundary                       string `json:",optional"`
	WireAPI                        string `json:",default=chat_completions"`
	Endpoint                       string
	APIKey                         string `json:",optional"`
	Model                          string
	TimeoutMs                      int64   `json:",default=90000,range=[1000:600000]"`
	MaxOutputTokens                int     `json:",default=32768,range=[1:65536]"`
	ContextWindowTokens            int     `json:",default=128000,range=[1000:2000000]"`
	PromptCostPerMillionTokens     float64 `json:",default=0"`
	CompletionCostPerMillionTokens float64 `json:",default=0"`
	CacheReadCostPerMillionTokens  float64 `json:",default=0"`
	CacheWriteCostPerMillionTokens float64 `json:",default=0"`
	ReasoningCostPerMillionTokens  float64 `json:",default=0"`
}

// SafetyConfig 是记忆写入的屏蔽词过滤配置。
type SafetyConfig struct {
	Enabled      bool `json:",default=true"`
	BlockedTerms []string
	MaxScanRunes int `json:",default=10000,range=[100:50000]"`
}

// WebSearchConfig 是网页搜索配置；APIKey 为空时 web_search 不可用。
type WebSearchConfig struct {
	Provider   string `json:",default=tavily"`
	APIKey     string `json:",optional"`
	Endpoint   string `json:",optional"`
	TimeoutMs  int64  `json:",default=8000,range=[100:30000]"`
	MaxResults int    `json:",default=5,range=[1:10]"`
}

// ElasticsearchConfig 是历史检索索引的连接配置。
type ElasticsearchConfig struct {
	Addresses []string
	Username  string `json:",optional"`
	Password  string `json:",optional"`
}

// BackgroundReviewConfig 指定后台记忆回顾使用的模型。
type BackgroundReviewConfig struct {
	Model string `json:",optional"`
}

// Config 是助手 worker 的配置。
type Config struct {
	service.ServiceConf
	InternalSecret   string
	DataSource       string
	Elasticsearch    ElasticsearchConfig
	SearchRpc        rpcx.RpcClientConf
	ContentRpc       rpcx.RpcClientConf
	MediaRpc         rpcx.RpcClientConf
	RecommendRpc     rpcx.RpcClientConf
	InteractionRpc   rpcx.RpcClientConf
	UserRpc          rpcx.RpcClientConf
	AllowedTools     []string
	LeaseSeconds     int `json:",default=60"`
	RenewSeconds     int `json:",default=10"`
	LLM              LLMConfig
	Safety           SafetyConfig
	WebSearch        WebSearchConfig
	BackgroundReview BackgroundReviewConfig
}
