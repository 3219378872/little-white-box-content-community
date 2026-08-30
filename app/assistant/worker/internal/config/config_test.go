package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestAgentConfigLoadsProviderReliabilitySettings(t *testing.T) {
	env := map[string]string{
		"RPC_INTERNAL_SECRET": "test-secret", "DB_ASSISTANT": "",
		"REDIS_HOST": "127.0.0.1:6379", "REDIS_PASSWORD": "",
		"ES_ADDRESS": "http://127.0.0.1:9200", "ES_USERNAME": "", "ES_PASSWORD": "",
		"ASSISTANT_LLM_ENABLED": "false", "ASSISTANT_LLM_WIRE_API": "responses",
		"ASSISTANT_LLM_ENDPOINT": "http://provider.test/v1", "ASSISTANT_LLM_API_KEY": "",
		"ASSISTANT_LLM_MODEL": "main-model", "ASSISTANT_LLM_MODEL_SMALL": "compact-model",
		"ASSISTANT_LLM_REVIEW_MODEL": "review-model", "TAVILY_API_KEY": "",
		"ASSISTANT_LLM_PROMPT_COST_PER_MILLION_TOKENS":               "1",
		"ASSISTANT_LLM_COMPLETION_COST_PER_MILLION_TOKENS":           "2",
		"ASSISTANT_LLM_CACHE_READ_COST_PER_MILLION_TOKENS":           "0.1",
		"ASSISTANT_LLM_CACHE_WRITE_COST_PER_MILLION_TOKENS":          "1.2",
		"ASSISTANT_LLM_REASONING_COST_PER_MILLION_TOKENS":            "3",
		"ASSISTANT_LLM_FALLBACK_ENABLED":                             "true",
		"ASSISTANT_LLM_FALLBACK_ROUTE_ID":                            "fallback",
		"ASSISTANT_LLM_FALLBACK_BOUNDARY":                            "default",
		"ASSISTANT_LLM_FALLBACK_WIRE_API":                            "chat_completions",
		"ASSISTANT_LLM_FALLBACK_ENDPOINT":                            "http://fallback.test/v1",
		"ASSISTANT_LLM_FALLBACK_API_KEY":                             "",
		"ASSISTANT_LLM_FALLBACK_MODEL":                               "fallback-model",
		"ASSISTANT_LLM_FALLBACK_PROMPT_COST_PER_MILLION_TOKENS":      "4",
		"ASSISTANT_LLM_FALLBACK_COMPLETION_COST_PER_MILLION_TOKENS":  "5",
		"ASSISTANT_LLM_FALLBACK_CACHE_READ_COST_PER_MILLION_TOKENS":  "0.4",
		"ASSISTANT_LLM_FALLBACK_CACHE_WRITE_COST_PER_MILLION_TOKENS": "1.4",
		"ASSISTANT_LLM_FALLBACK_REASONING_COST_PER_MILLION_TOKENS":   "6",
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	var cfg Config
	if err := conf.Load("../../etc/agent.yaml", &cfg, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if cfg.LLM.RouteID != "primary" || cfg.LLM.Boundary != "default" || cfg.LLM.AuxModel != "compact-model" ||
		cfg.BackgroundReview.Model != "review-model" || !cfg.LLM.CanaryEnabled || cfg.LLM.RetryMaxAttempts != 3 {
		t.Fatalf("provider reliability config=%+v review=%+v", cfg.LLM, cfg.BackgroundReview)
	}
	if cfg.LLM.CacheReadCostPerMillionTokens != 0.1 || cfg.LLM.CacheWriteCostPerMillionTokens != 1.2 ||
		cfg.LLM.ReasoningCostPerMillionTokens != 3 {
		t.Fatalf("usage prices=%+v", cfg.LLM)
	}
	if len(cfg.LLM.Fallbacks) != 1 || !cfg.LLM.Fallbacks[0].Enabled ||
		cfg.LLM.Fallbacks[0].RouteID != "fallback" || cfg.LLM.Fallbacks[0].Model != "fallback-model" ||
		cfg.LLM.Fallbacks[0].CacheReadCostPerMillionTokens != 0.4 {
		t.Fatalf("fallback config=%+v", cfg.LLM.Fallbacks)
	}
	t.Setenv("ASSISTANT_LLM_FALLBACK_ENABLED", "false")
	var disabled Config
	if err := conf.Load("../../etc/agent.yaml", &disabled, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if len(disabled.LLM.Fallbacks) != 1 || disabled.LLM.Fallbacks[0].Enabled {
		t.Fatalf("disabled fallback config=%+v", disabled.LLM.Fallbacks)
	}
}
