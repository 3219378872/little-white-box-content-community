package config

import (
	"testing"

	conf "esx/pkg/configx"
)

// Transport privacy is enforced centrally by rpcx and covered with real RPC calls.
func TestContentConfigLoadsNativeTransportPolicy(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("REDIS_PASS", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/content.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if c.Name != "content.rpc" || !c.Health || c.MaxConnections <= 0 || c.Timeout <= 0 {
		t.Fatalf("invalid native transport policy: name=%s health=%v timeout=%d", c.Name, c.Health, c.Timeout)
	}
}
