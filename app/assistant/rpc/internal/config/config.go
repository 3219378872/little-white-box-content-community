package config

import (
	"github.com/zeromicro/go-zero/zrpc"
)

type LLMConfig struct {
	Enabled                        bool
	WireAPI                        string `json:",default=chat_completions"`
	Endpoint                       string
	APIKey                         string `json:",optional"`
	Model                          string
	TimeoutMs                      int64   `json:",default=8000,range=[100:60000]"`
	MaxContextRunes                int     `json:",default=8000,range=[100:50000]"`
	MaxOutputRunes                 int     `json:",default=8000,range=[100:50000]"`
	MaxOutputTokens                int     `json:",default=4096,range=[1:32768]"`
	PromptCostPerMillionTokens     float64 `json:",default=0"`
	CompletionCostPerMillionTokens float64 `json:",default=0"`
}

type SafetyConfig struct {
	Enabled      bool `json:",default=true"`
	BlockedTerms []string
	MaxScanRunes int `json:",default=10000,range=[100:50000]"`
}

type Config struct {
	zrpc.RpcServerConf
	SearchRpc               zrpc.RpcClientConf
	ContentRpc              zrpc.RpcClientConf
	RecommendRpc            zrpc.RpcClientConf
	UserRpc                 zrpc.RpcClientConf
	AllowedTools            []string
	ToolTimeoutMs           int64  `json:",default=1500,range=[1:10000]"`
	MaxMessageRunes         int    `json:",default=2000,range=[1:10000]"`
	MaxSources              int    `json:",default=5,range=[1:20]"`
	TokenChunkRunes         int    `json:",default=64,range=[1:500]"`
	StateKeyPrefix          string `json:",default=assistant:v2"`
	ConversationTTLSeconds  int    `json:",default=2592000,range=[3600:31536000]"`
	ConversationMaxMessages int    `json:",default=100,range=[2:1000]"`
	QuotaWindowSeconds      int    `json:",default=60,range=[1:86400]"`
	QuotaRequests           int    `json:",default=20,range=[1:10000]"`
	LLM                     LLMConfig
	Safety                  SafetyConfig
}
