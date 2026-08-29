package config

import (
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type LLMConfig struct {
	Enabled                        bool
	WireAPI                        string `json:",default=chat_completions"`
	Endpoint                       string
	APIKey                         string `json:",optional"`
	Model                          string
	TimeoutMs                      int64   `json:",default=90000,range=[1000:600000]"`
	MaxOutputTokens                int     `json:",default=32768,range=[1:65536]"`
	ContextWindowTokens            int     `json:",default=128000,range=[1000:2000000]"`
	PromptCostPerMillionTokens     float64 `json:",default=0"`
	CompletionCostPerMillionTokens float64 `json:",default=0"`
}

type SafetyConfig struct {
	Enabled      bool `json:",default=true"`
	BlockedTerms []string
	MaxScanRunes int `json:",default=10000,range=[100:50000]"`
}

type WebSearchConfig struct {
	Provider   string `json:",default=tavily"`
	APIKey     string `json:",optional"`
	Endpoint   string `json:",optional"`
	TimeoutMs  int64  `json:",default=8000,range=[100:30000]"`
	MaxResults int    `json:",default=5,range=[1:10]"`
}

type ElasticsearchConfig struct {
	Addresses []string
	Username  string `json:",optional"`
	Password  string `json:",optional"`
}

type BackgroundReviewConfig struct {
	Model string `json:",optional"`
}

type Config struct {
	service.ServiceConf
	InternalSecret   string
	DataSource       string
	Redis            redis.RedisKeyConf
	Elasticsearch    ElasticsearchConfig
	SearchRpc        zrpc.RpcClientConf
	ContentRpc       zrpc.RpcClientConf
	MediaRpc         zrpc.RpcClientConf
	RecommendRpc     zrpc.RpcClientConf
	InteractionRpc   zrpc.RpcClientConf
	UserRpc          zrpc.RpcClientConf
	AllowedTools     []string
	LeaseSeconds     int `json:",default=60"`
	RenewSeconds     int `json:",default=10"`
	LLM              LLMConfig
	Safety           SafetyConfig
	WebSearch        WebSearchConfig
	BackgroundReview BackgroundReviewConfig
}
