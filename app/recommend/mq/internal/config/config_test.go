package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestConsumerConfigInjectsAuthoritativePreferenceService(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("MQ_NAMESERVER", "127.0.0.1:9876")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "")
	var c Config
	require.NoError(t, conf.Load("../../etc/recommend-consumer.yaml", &c, conf.UseEnv()))
	require.NoError(t, c.Validate())
	require.Equal(t, "user.rpc", c.UserRpc.Etcd.Key)
	require.Equal(t, "test-internal-secret", c.InternalSecret)
	c.InternalSecret = ""
	require.Error(t, c.Validate())
}
