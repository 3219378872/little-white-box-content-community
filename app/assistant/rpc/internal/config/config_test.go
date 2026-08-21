package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestAssistantConfigEnablesFrameworkHealthAndMetrics(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("ASSISTANT_LLM_WIRE_API", "responses")
	t.Setenv("ASSISTANT_LLM_ENDPOINT", "http://127.0.0.1:18080/v1/responses")
	t.Setenv("ASSISTANT_LLM_API_KEY", "")
	t.Setenv("ASSISTANT_LLM_MODEL", "test-model")
	t.Setenv("ASSISTANT_LLM_ENABLED", "true")
	t.Setenv("ASSISTANT_LLM_PROMPT_COST_PER_MILLION_TOKENS", "1.25")
	t.Setenv("ASSISTANT_LLM_COMPLETION_COST_PER_MILLION_TOKENS", "5")
	var c Config
	if err := conf.Load("../../etc/assistant.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if !c.Health || !c.DevServer.Enabled || !c.DevServer.EnableMetrics || c.DevServer.EnablePprof || c.DevServer.Port != 9124 {
		t.Fatalf("unexpected observability config: health=%v devserver=%+v", c.Health, c.DevServer)
	}
	if !c.Safety.Enabled || c.Safety.MaxScanRunes != 10000 || len(c.Safety.BlockedTerms) < 2 {
		t.Fatalf("unexpected safety config: %+v", c.Safety)
	}
	if c.LLM.PromptCostPerMillionTokens != 1.25 || c.LLM.CompletionCostPerMillionTokens != 5 {
		t.Fatalf("unexpected LLM cost config: %+v", c.LLM)
	}
	if !c.LLM.Enabled {
		t.Fatal("LLM environment switch was not loaded")
	}
	if c.LLM.WireAPI != "responses" || c.LLM.MaxOutputTokens != 4096 {
		t.Fatalf("unexpected LLM protocol config: %+v", c.LLM)
	}
}

// REL-022：assistant 必须抑制框架自动内容日志（Chat 携带全量用户输入）。
func TestAssistantConfigSuppressesChatContentLogging(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("ASSISTANT_LLM_WIRE_API", "responses")
	t.Setenv("ASSISTANT_LLM_ENDPOINT", "http://127.0.0.1:18080/v1/responses")
	t.Setenv("ASSISTANT_LLM_API_KEY", "")
	t.Setenv("ASSISTANT_LLM_MODEL", "test-model")
	t.Setenv("ASSISTANT_LLM_ENABLED", "true")
	t.Setenv("ASSISTANT_LLM_PROMPT_COST_PER_MILLION_TOKENS", "1.25")
	t.Setenv("ASSISTANT_LLM_COMPLETION_COST_PER_MILLION_TOKENS", "5")
	var c Config
	if err := conf.Load("../../etc/assistant.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	methods := c.Middlewares.StatConf.IgnoreContentMethods
	if len(methods) != 1 || methods[0] != "/assistant.AssistantService/Chat" {
		t.Fatalf("expected Chat to be ignored from content logging, got %v", methods)
	}
}
