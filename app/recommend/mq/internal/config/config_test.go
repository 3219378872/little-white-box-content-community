package config

import (
	"testing"

	conf "esx/pkg/configx"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, 7776000, c.DedupTTL)
	require.Equal(t, 2592000, c.FeatureTTL)
	c.InternalSecret = ""
	require.Error(t, c.Validate())
}

func TestFeatureAndDedupRetentionDefaultsAreIndependent(t *testing.T) {
	var c Config
	require.NoError(t, conf.FillDefault(&c))
	require.Equal(t, 2592000, c.FeatureTTL)
	require.Equal(t, 7776000, c.DedupTTL)
	require.Equal(t, 2592000, c.LegacyDedupTTL)
	c.InternalSecret = "test"
	require.NoError(t, c.Validate())
	c.FeatureTTL = 3600
	require.NoError(t, c.Validate())
	require.Equal(t, 7776000, c.DedupTTL)
	c.DedupTTL = 2592000
	require.Error(t, c.Validate())
}

func TestDedupMigrationRejectsClusterConfiguration(t *testing.T) {
	var c Config
	require.NoError(t, conf.FillDefault(&c))
	c.InternalSecret = "test-internal-secret"
	for _, mode := range []string{"", "node"} {
		c.Redis.Type = mode
		require.NoError(t, c.Validate())
	}
	c.Redis.Type = "cluster"
	require.ErrorContains(t, c.Validate(), "cluster-wide dedup retention migration is not supported")
}
