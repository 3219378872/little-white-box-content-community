package main

import (
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestCommandConfigLoadsSharedSearchYAML(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("ES_ADDRESS", "http://127.0.0.1:9200")
	t.Setenv("ES_USERNAME", "")
	t.Setenv("ES_PASSWORD", "")
	t.Setenv("MQ_NAMESERVER", "127.0.0.1:9876")
	var c commandConfig
	if err := conf.Load("../../etc/search-consumer.yaml", &c, conf.UseEnv()); err != nil {
		t.Fatal(err)
	}
	if c.ES.Index != "xbh_posts" || c.Rebuild.PageSize != 50 || c.Rebuild.TimeoutSeconds != 900 {
		t.Fatalf("unexpected config: %+v", c)
	}
	if c.ContentRpc.Etcd.Key != "content.rpc" {
		t.Fatalf("content rpc key=%q", c.ContentRpc.Etcd.Key)
	}
}
