package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

// REL-022：user RPC 必须抑制框架自动内容日志（认证/资料字段）。
func TestUserConfigSuppressesContentLogging(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("JWT_SECRET_KEY", "test-jwt-secret")
	t.Setenv("REDIS_PASS", "")
	t.Setenv("DB_USER", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/user.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	methods := c.Middlewares.StatConf.IgnoreContentMethods
	if len(methods) == 0 {
		t.Fatal("user config must set IgnoreContentMethods")
	}
	seen := make(map[string]bool, len(methods))
	for _, method := range methods {
		seen[method] = true
	}
	if !seen["/user.UserService/Login"] || !seen["/user.UserService/Register"] {
		t.Errorf("user config must ignore content for Login/Register, got %v", methods)
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
	valid.InternalSecret = "test-internal-secret"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() rejected configured JwtConfig.AccessSecret: %v", err)
	}
}
