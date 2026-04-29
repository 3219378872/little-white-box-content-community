package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/media/mq/internal/svc"
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

// ObjectDeleter is the minimal interface for S3 deletion used by the consumer.
type ObjectDeleter interface {
	Delete(ctx context.Context, objectKey string) error
}

func NewMediaCleanupConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("media-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeMediaDeleteBatch(ctx, svcCtx.Storage, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicMediaDelete, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("media-consumer: subscribe %s: %w", mqx.TopicMediaDelete, err)
	}
	return c, nil
}

func consumeMediaDeleteBatch(ctx context.Context, deleter ObjectDeleter, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var m mediaDeletedMessage
		if err := json.Unmarshal(msg.Body, &m); err != nil {
			logx.WithContext(ctx).Errorw("media-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if m.S3ObjectKey == "" {
			logx.WithContext(ctx).Errorw("media-consumer: empty s3_object_key, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("media_id", m.MediaId))
			continue
		}
		if err := deleter.Delete(ctx, m.S3ObjectKey); err != nil {
			logx.WithContext(ctx).Errorw("media-consumer: delete s3 object failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("media_id", m.MediaId),
				logx.Field("object_key", m.S3ObjectKey), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		logx.WithContext(ctx).Infow("media-consumer: s3 object deleted",
			logx.Field("media_id", m.MediaId), logx.Field("object_key", m.S3ObjectKey))
	}
	return consumer.ConsumeSuccess
}
