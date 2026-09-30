package svc

import (
	"testing"

	"esx/app/recommend/mq/internal/config"
	conf "esx/pkg/configx"
	"esx/pkg/redisstore"

	"github.com/stretchr/testify/require"
)

func TestClusterCannotReachMigrationReadyOrConsumerStartup(t *testing.T) {
	var c config.Config
	require.NoError(t, conf.FillDefault(&c))
	c.InternalSecret = "test-internal-secret"
	c.Redis.Type = redisstore.ClusterType
	// Leave every external dependency unconfigured. The exact validation failure
	// proves startup stops before creating RPC/Redis clients or a migrator.
	require.PanicsWithError(t,
		"recommend consumer requires Redis Type=node: cluster-wide dedup retention migration is not supported",
		func() { NewServiceContext(c) },
	)
}
