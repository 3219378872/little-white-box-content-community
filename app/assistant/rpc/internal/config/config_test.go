package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestAssistantConfigEnablesFrameworkHealthAndMetrics(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("DB_ASSISTANT", "")
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
}

func TestAssistantConfigSuppressesPostMessageContentLogging(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("DB_ASSISTANT", "")
	var c Config
	if err := conf.Load("../../etc/assistant.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	methods := c.Middlewares.StatConf.IgnoreContentMethods
	want := map[string]bool{
		"/assistant.AssistantService/PostMessage":        false,
		"/assistant.AssistantService/SubscribeRunEvents": false,
	}
	for _, method := range methods {
		if _, ok := want[method]; ok {
			want[method] = true
		}
	}
	for method, ok := range want {
		if !ok {
			t.Fatalf("expected %s to be ignored from content logging, got %v", method, methods)
		}
	}
}
