package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/feed/mq/internal/logic"
	"esx/app/feed/mq/internal/svc"
	"esx/pkg/event"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

// NewPostPublishConsumer 订阅 post-create，按 PostEvent 触发 inbox/outbox fanout。
// spec §3.2 feed-fanout-consumer，与 search/embedding/cleanup 统一使用 pkg/event.PostEvent。
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var e event.PostEvent
		if err := json.Unmarshal(msg.Body, &e); err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if err := e.Validate(); err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: invalid event, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if e.Type != event.PostEventCreated {
			// 只处理新发布；编辑/删除有其他消费者负责
			continue
		}
		_, err := logic.HandlePostPublished(ctx,
			svcCtx.OutboxModel, svcCtx.InboxModel, svcCtx.UserService,
			svcCtx.BigVThreshold, svcCtx.FanoutBatchSize,
			logic.PostPublished{
				PostId: e.PostID, AuthorId: e.AuthorID, CreatedAt: e.EventTime,
			})
		if err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: fanout failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
