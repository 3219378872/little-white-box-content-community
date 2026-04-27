package mqs

import (
	"context"
	"encoding/json"
	"esx/app/media/rpc/internal/svc"
	"fmt"

	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type mediaDeletedMessage struct {
	MediaId     int64  `json:"media_id"`
	S3ObjectKey string `json:"s3_object_key"`
	Bucket      string `json:"bucket"`
	DeletedAt   int64  `json:"deleted_at"`
}

// NewMediaCleanupConsumer 创建并注册媒体清理消费者。
func NewMediaCleanupConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	cfg := mqx.ConsumerConfig{
		NameServer: svcCtx.Config.MQ.NameServer,
		GroupName:  mqx.GroupMediaService,
	}

	c, err := mqx.NewConsumer(cfg)
	if err != nil {
		return nil, fmt.Errorf("create media cleanup consumer: %w", err)
	}

	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			var m mediaDeletedMessage
			if err := json.Unmarshal(msg.Body, &m); err != nil {
				logx.WithContext(ctx).Errorw("unmarshal media_deleted message failed",
					logx.Field("msg_id", msg.MsgId),
					logx.Field("err", err.Error()),
				)
				continue
			}

			if m.S3ObjectKey == "" {
				logx.WithContext(ctx).Errorw("media_deleted message has empty object key, skipping",
					logx.Field("media_id", m.MediaId),
					logx.Field("msg_id", msg.MsgId),
				)
				continue
			}

			if err := svcCtx.Storage.Delete(ctx, m.S3ObjectKey); err != nil {
				logx.WithContext(ctx).Errorw("delete s3 object failed",
					logx.Field("media_id", m.MediaId),
					logx.Field("object_key", m.S3ObjectKey),
					logx.Field("err", err.Error()),
				)
				return consumer.ConsumeRetryLater, nil
			}

			logx.WithContext(ctx).Infow("s3 object deleted",
				logx.Field("media_id", m.MediaId),
				logx.Field("object_key", m.S3ObjectKey),
			)
		}
		return consumer.ConsumeSuccess, nil
	}

	if err := c.SubscribeWithTopic(mqx.TopicMediaDelete, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("subscribe media_deleted topic: %w", err)
	}

	return c, nil
}
