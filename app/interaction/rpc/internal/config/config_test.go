package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

// REL-022：interaction RPC 必须抑制框架自动内容日志。
func TestInteractionConfigSuppressesContentLogging(t *testing.T) {
	t.Setenv("REDIS_PASS", "")
	t.Setenv("DB_INTERACTION", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/interaction.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if len(c.Middlewares.StatConf.IgnoreContentMethods) == 0 {
		t.Fatal("interaction config must set IgnoreContentMethods")
	}
}
