package config

import (
	"testing"

	conf "esx/pkg/configx"
)

// Transport privacy is enforced centrally by rpcx and covered with real RPC calls.
func TestUserConfigLoadsNativeTransportPolicy(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("JWT_SECRET_KEY", "test-jwt-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	t.Setenv("REDIS_PASS", "")
	t.Setenv("DB_USER", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/user.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if c.Name != "user.rpc" || !c.Health || c.MaxConnections <= 0 || c.Timeout <= 0 {
		t.Fatalf("invalid native transport policy: name=%s health=%v timeout=%d", c.Name, c.Health, c.Timeout)
	}
}

func TestUserConfigValidateRequiresJwtSecret(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	for _, secret := range []string{"", " \t\n"} {
		c := Config{}
		c.JwtConfig.AccessSecret = secret
		if err := c.Validate(); err == nil {
			t.Fatalf("Validate() accepted blank JwtConfig.AccessSecret %q", secret)
		}
	}

	valid := Config{}
	valid.JwtConfig.AccessSecret = "user-jwt-secret"
	valid.JwtConfig.RefreshSecret = "user-refresh-secret"
	valid.InternalSecret = "test-internal-secret"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() rejected configured JwtConfig.AccessSecret: %v", err)
	}
}
