package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"esx/pkg/mqx"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestCountSyncConsumerConfigRequiresDistinctGroup(t *testing.T) {
	t.Parallel()

	base := Config{
		MQ: mqx.ConsumerConfig{
			NameServer:        "127.0.0.1:9876",
			GroupName:         mqx.GroupContentCleanup,
			MaxReconsumeTimes: 8,
		},
	}

	t.Run("missing group", func(t *testing.T) {
		t.Parallel()
		_, err := base.CountSyncConsumerConfig()
		require.Error(t, err)
		require.Contains(t, err.Error(), "CountSync.GroupName is required")
	})

	t.Run("shared group", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.CountSync.GroupName = mqx.GroupContentCleanup
		_, err := cfg.CountSyncConsumerConfig()
		require.Error(t, err)
		require.Contains(t, err.Error(), "must differ")
	})

	t.Run("inherits nameserver and retries", func(t *testing.T) {
		t.Parallel()
		cfg := base
		cfg.CountSync.GroupName = mqx.GroupContentCountSync
		got, err := cfg.CountSyncConsumerConfig()
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1:9876", got.NameServer)
		require.Equal(t, int32(8), got.MaxReconsumeTimes)
		require.Equal(t, mqx.GroupContentCountSync, got.GroupName)
	})
}

func TestContentCleanupYamlDeclaresDistinctCountSyncGroup(t *testing.T) {
	t.Setenv("MQ_NAMESERVER", "127.0.0.1:9876")
	t.Setenv("DB_CONTENT", "user:pass@tcp(127.0.0.1:3306)/xbh_content")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "test")

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(thisFile), "../../etc/content-cleanup.yaml")

	var cfg Config
	require.NoError(t, conf.Load(path, &cfg, conf.UseEnv()))
	require.Equal(t, mqx.GroupContentCleanup, cfg.MQ.GroupName)
	require.Equal(t, mqx.GroupContentCountSync, cfg.CountSync.GroupName)
	require.Equal(t, int32(8), cfg.CountSync.MaxReconsumeTimes)

	got, err := cfg.CountSyncConsumerConfig()
	require.NoError(t, err)
	require.Equal(t, mqx.GroupContentCountSync, got.GroupName)
	require.NotEqual(t, cfg.MQ.GroupName, got.GroupName)
}
