package config

import (
	"testing"

	conf "esx/pkg/configx"
)

func TestAssistantConfigEnablesFrameworkHealthAndMetrics(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
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

func TestAssistantConfigLoadsNativeTransportPolicy(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("DB_ASSISTANT", "")
	var c Config
	if err := conf.Load("../../etc/assistant.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if c.Name != "assistant.rpc" || !c.Health || c.MaxConnections <= 0 || c.Timeout <= 0 {
		t.Fatalf("invalid native transport policy: name=%s health=%v timeout=%d", c.Name, c.Health, c.Timeout)
	}
}
