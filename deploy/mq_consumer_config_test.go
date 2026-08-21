package deploy

import (
	"path/filepath"
	"testing"

	"mqx"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
)

func TestAllConsumerConfigsDeclareBoundedRetries(t *testing.T) {
	t.Setenv("MQ_NAMESERVER", "127.0.0.1:9876")
	files := []string{
		"app/content/mq/cleanup/etc/content-cleanup.yaml",
		"app/embedding/mq/etc/embedding-consumer.yaml",
		"app/feed/mq/etc/feed-consumer.yaml",
		"app/feed/rpc/etc/feed.yaml",
		"app/media/mq/etc/media-consumer.yaml",
		"app/pipeline/behaviorlog/etc/behavior-log.yaml",
		"app/recommend/mq/etc/recommend-consumer.yaml",
		"app/search/mq/etc/search-consumer.yaml",
	}
	for _, file := range files {
		file := file
		t.Run(file, func(t *testing.T) {
			var config struct {
				MQ mqx.ConsumerConfig
			}
			path := filepath.Join("..", file)
			require.NoError(t, conf.Load(path, &config, conf.UseEnv()))
			require.Equal(t, int32(8), config.MQ.MaxReconsumeTimes)
		})
	}
}

func TestContentCleanupCountSyncDeclaresBoundedRetriesAndDistinctGroup(t *testing.T) {
	t.Setenv("MQ_NAMESERVER", "127.0.0.1:9876")
	t.Setenv("DB_CONTENT", "user:pass@tcp(127.0.0.1:3306)/xbh_content")
	t.Setenv("REDIS_HOST", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "test")

	var config struct {
		MQ        mqx.ConsumerConfig
		CountSync mqx.ConsumerConfig
	}
	require.NoError(t, conf.Load(filepath.Join("..", "app/content/mq/cleanup/etc/content-cleanup.yaml"), &config, conf.UseEnv()))
	require.Equal(t, int32(8), config.CountSync.MaxReconsumeTimes)
	require.Equal(t, mqx.GroupContentCountSync, config.CountSync.GroupName)
	require.NotEqual(t, config.MQ.GroupName, config.CountSync.GroupName)
}
