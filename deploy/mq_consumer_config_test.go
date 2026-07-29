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
		"app/message/mq/etc/message-consumer.yaml",
		"app/message/rpc/etc/message.yaml",
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
