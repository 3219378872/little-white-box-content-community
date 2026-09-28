package config

import (
	"testing"

	conf "esx/pkg/configx"
)

// Transport privacy is enforced centrally by rpcx and covered with real RPC calls.
func TestInteractionConfigLoadsNativeTransportPolicy(t *testing.T) {
	t.Setenv("REDIS_PASS", "")
	t.Setenv("DB_INTERACTION", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/interaction.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if c.Name != "interaction.rpc" || !c.Health || c.MaxConnections <= 0 || c.Timeout <= 0 {
		t.Fatalf("invalid native transport policy: name=%s health=%v timeout=%d", c.Name, c.Health, c.Timeout)
	}
}
