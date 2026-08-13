package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

// REL-022：media RPC 必须抑制框架自动内容日志。
func TestMediaConfigSuppressesContentLogging(t *testing.T) {
	t.Setenv("REDIS_PASS", "")
	t.Setenv("DB_MEDIA", "")
	t.Setenv("S3_ACCESS_KEY", "k")
	t.Setenv("S3_SECRET_KEY", "s")
	t.Setenv("MQ_NAMESERVER", "")
	var c Config
	if err := conf.Load("../../etc/media.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if len(c.Middlewares.StatConf.IgnoreContentMethods) == 0 {
		t.Fatal("media config must set IgnoreContentMethods")
	}
}
