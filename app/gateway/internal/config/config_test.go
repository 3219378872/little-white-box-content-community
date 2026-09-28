package config

import (
	"testing"

	conf "esx/pkg/configx"
)

func TestConfigValidateRequiresAccessSecret(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	for _, secret := range []string{"", " \t\n"} {
		c := Config{}
		c.Auth.AccessSecret = secret
		if err := c.Validate(); err == nil {
			t.Fatalf("Validate() accepted blank AccessSecret %q", secret)
		}
	}

	valid := Config{}
	valid.Auth.AccessSecret = "gateway-jwt-secret"
	valid.InternalSecret = "test-internal-secret"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() rejected configured AccessSecret: %v", err)
	}
}

func TestGatewayYAMLLoadsSecretFromEnvironment(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("JWT_SECRET_KEY", "configured-gateway-jwt-secret")

	var c Config
	if err := conf.Load("../../etc/gateway.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatalf("load gateway.yaml: %v", err)
	}
	if c.Auth.AccessSecret != "configured-gateway-jwt-secret" {
		t.Fatalf("AccessSecret = %q", c.Auth.AccessSecret)
	}
	if c.RestConf.Port != 8888 || c.RestConf.MaxBytes != 10<<20 || c.RestConf.Timeout != 3000 {
		t.Fatal("gateway HTTP configuration changed")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() rejected loaded config: %v", err)
	}
}
