package config

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestConfigValidate(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	valid := Config{
		CursorSecret:    "0123456789abcdef0123456789abcdef",
		DefaultPageSize: 20,
		MaxPageSize:     50,
		ExploreRatio:    0.1,
		InternalSecret:  "test-internal-secret",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	invalid := valid
	invalid.CursorSecret = "short"
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted a short cursor secret")
	}
	invalid = valid
	invalid.ExploreRatio = 0.8
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted an excessive exploration ratio")
	}
	invalid = valid
	invalid.OnlineInfer.Enabled = true
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() accepted enabled inference without an endpoint")
	}
}

func TestRecommendYAMLLoadsEnvironmentAndDefaults(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("RECOMMEND_CURSOR_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ES_ADDRESS", "http://127.0.0.1:9200")
	t.Setenv("ES_USERNAME", "")
	t.Setenv("ES_PASSWORD", "")
	t.Setenv("MILVUS_ADDRESS", "127.0.0.1:19530")
	t.Setenv("MILVUS_USERNAME", "")
	t.Setenv("MILVUS_PASSWORD", "")
	t.Setenv("MILVUS_DATABASE", "")
	t.Setenv("ONLINE_INFER_ENDPOINT", "127.0.0.1:9025")
	var c Config
	if err := conf.Load("../../etc/recommend.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatalf("load recommend.yaml: %v", err)
	}
	if c.ListenOn != "0.0.0.0:9023" || c.FeatureVersion != "v2" || c.DefaultPageSize != 20 || c.OnlineInfer.TimeoutMs != 80 {
		t.Fatalf("unexpected config: %+v", c)
	}
	if !c.Health || !c.DevServer.Enabled || !c.DevServer.EnableMetrics || c.DevServer.EnablePprof || c.DevServer.Port != 9123 {
		t.Fatalf("unexpected observability config: health=%v devserver=%+v", c.Health, c.DevServer)
	}
	if !c.OnlineInfer.Enabled || len(c.OnlineInfer.Rpc.Endpoints) != 1 || c.OnlineInfer.Rpc.Endpoints[0] != "127.0.0.1:9025" {
		t.Fatalf("unexpected OnlineInfer config: %+v", c.OnlineInfer)
	}
	if c.OnlineInfer.ModelVersion != "auto" {
		t.Fatalf("recommend must follow OnlineInfer activation and rollback, got model %q", c.OnlineInfer.ModelVersion)
	}
	if c.ElasticsearchRecall.Index != "xbh_posts" {
		t.Fatalf("recommend must use the active search alias, got %q", c.ElasticsearchRecall.Index)
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
