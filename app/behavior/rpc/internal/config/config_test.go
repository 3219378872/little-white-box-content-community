package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestBehaviorConfigEnablesFrameworkHealthAndMetrics(t *testing.T) {
	t.Setenv("MQ_NAMESERVER", "127.0.0.1:9876")
	var c Config
	if err := conf.Load("../../etc/behavior.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if !c.Health || !c.DevServer.Enabled || !c.DevServer.EnableMetrics || c.DevServer.EnablePprof || c.DevServer.Port != 9121 {
		t.Fatalf("unexpected observability config: health=%v devserver=%+v", c.Health, c.DevServer)
	}
}
