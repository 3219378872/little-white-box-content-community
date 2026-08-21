package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestSearchConfigLoadsUserServiceDependency(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("ES_ADDRESS", "http://127.0.0.1:9200")
	t.Setenv("ES_USERNAME", "")
	t.Setenv("ES_PASSWORD", "")
	var config Config
	require.NoError(t, conf.Load("../../etc/search.yaml", &config, conf.UseEnv()))
	require.Equal(t, "xbh_posts", config.ES.Index)
	require.Equal(t, "user.rpc", config.UserRpc.Etcd.Key)
	require.True(t, config.Health)
	require.True(t, config.DevServer.Enabled)
	require.True(t, config.DevServer.EnableMetrics)
	require.False(t, config.DevServer.EnablePprof)
	require.Equal(t, 9122, config.DevServer.Port)
}
