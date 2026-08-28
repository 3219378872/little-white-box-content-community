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
	ModelSmall                     string  `json:",optional"`
	TimeoutMs                      int64   `json:",default=8000,range=[100:60000]"`
	MaxContextRunes                int     `json:",default=8000,range=[100:50000]"`
	MaxOutputRunes                 int     `json:",default=8000,range=[100:50000]"`
	MaxOutputTokens                int     `json:",default=32768,range=[1:32768]"`
	PromptCostPerMillionTokens     float64 `json:",default=0"`
	CompletionCostPerMillionTokens float64 `json:",default=0"`
}

type SafetyConfig struct {
	Enabled      bool `json:",default=true"`
	BlockedTerms []string
	MaxScanRunes int `json:",default=10000,range=[100:50000]"`
}

// AgentConfig 是 Agent 模式的编排预算与工具配置（SPEC-assistant-agent-mode）。
// MaxStepsSoft/MaxStepsHard 为软/硬上限：超软限后在工具结果注入剩余轮数通知
// （AGNT-030），达硬限剥离工具强制收尾（AGNT-031）。
type AgentConfig struct {
	Enabled             bool  `json:",default=false"`
	MaxStepsSoft        int   `json:",default=8,range=[1:50]"`
	MaxStepsHard        int   `json:",default=12,range=[2:100]"`
	MaxToolCallsPerTurn int   `json:",default=12,range=[1:100]"`
	TurnTimeoutMs       int64 `json:",default=300000,range=[1000:600000]"`
	StepTimeoutMs       int64 `json:",default=90000,range=[1000:120000]"`
	ConfirmTimeoutSecs  int   `json:",default=120,range=[5:600]"`
	QuotaRequests       int   `json:",default=10,range=[1:10000]"` // 独立配额，与 enhanced_search 分开计量（AGNT-032）
	AllowedTools        []string
	SystemPrompt        string `json:",optional"`
	WebSearch           WebSearchConfig
}

type WebSearchConfig struct {
	Provider   string `json:",default=tavily"`
	APIKey     string `json:",optional"`
	Endpoint   string `json:",optional"`
	TimeoutMs  int64  `json:",default=8000,range=[100:30000]"`
	MaxResults int    `json:",default=5,range=[1:10]"`
}

type Config struct {
	zrpc.RpcServerConf
	InternalSecret          string
	DataSource              string `json:",optional"`
	SearchRpc               zrpc.RpcClientConf
	ContentRpc              zrpc.RpcClientConf
	MediaRpc                zrpc.RpcClientConf
	RecommendRpc            zrpc.RpcClientConf
	InteractionRpc          zrpc.RpcClientConf
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
	Agent                   AgentConfig
	LLM                     LLMConfig
	Safety                  SafetyConfig
}
