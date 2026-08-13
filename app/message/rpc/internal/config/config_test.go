package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

// REL-022：message RPC 必须抑制框架自动内容日志（请求携带私信正文/通知内容）。
func TestMessageConfigSuppressesContentLogging(t *testing.T) {
	t.Setenv("REDIS_PASS", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/message.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	methods := c.Middlewares.StatConf.IgnoreContentMethods
	if len(methods) == 0 {
		t.Fatal("message config must set IgnoreContentMethods")
	}
	seen := make(map[string]bool, len(methods))
	for _, method := range methods {
		seen[method] = true
	}
	for _, required := range []string{
		"/message.MessageService/SendMessage",
		"/message.MessageService/GetMessages",
	} {
		if !seen[required] {
			t.Errorf("message config must ignore content for %s", required)
		}
	}
}
