package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/feed/mq/internal/logic"
	"esx/app/feed/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type postPublishedMessage struct {
	PostId    int64 `json:"post_id"`
	AuthorId  int64 `json:"author_id"`
	CreatedAt int64 `json:"created_at"`
}

func NewPostPublishConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("feed-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeMessageBatch(ctx, svcCtx, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicPostCreate, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("feed-consumer: subscribe %s: %w", mqx.TopicPostCreate, err)
	}
	return c, nil
}

func consumeMessageBatch(ctx context.Context, svcCtx *svc.ServiceContext, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event postPublishedMessage
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.PostId <= 0 || event.AuthorId <= 0 || event.CreatedAt <= 0 {
			logx.WithContext(ctx).Errorw("feed-consumer: missing required fields",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", event.PostId),
				logx.Field("author_id", event.AuthorId), logx.Field("created_at", event.CreatedAt))
			continue
		}
		_, err := logic.HandlePostPublished(ctx,
			svcCtx.OutboxModel, svcCtx.InboxModel, svcCtx.UserService,
			svcCtx.BigVThreshold, svcCtx.FanoutBatchSize,
			logic.PostPublished{
				PostId: event.PostId, AuthorId: event.AuthorId, CreatedAt: event.CreatedAt,
			})
		if err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: fanout failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", event.PostId),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
