package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

// REL-022：content RPC 必须抑制框架自动内容日志（请求携带社区正文/评论）。
func TestContentConfigSuppressesContentLogging(t *testing.T) {
	t.Setenv("REDIS_PASS", "")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/content.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	methods := c.Middlewares.StatConf.IgnoreContentMethods
	if len(methods) == 0 {
		t.Fatal("content config must set IgnoreContentMethods")
	}
	seen := make(map[string]bool, len(methods))
	for _, method := range methods {
		seen[method] = true
	}
	for _, required := range []string{
		"/content.ContentService/CreatePost",
		"/content.ContentService/UpdatePost",
		"/content.ContentService/CreateComment",
	} {
		if !seen[required] {
			t.Errorf("content config must ignore content for %s", required)
		}
	}
}
