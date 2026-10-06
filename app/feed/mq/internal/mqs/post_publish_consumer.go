package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/feed/mq/internal/logic"
	"esx/app/feed/mq/internal/svc"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/visibilityx"

	"esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
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
	// CORE-015：草稿→发布转换（post-update 且 status=1）也需要 fanout。
	if err := c.SubscribeWithTopic(mqx.TopicPostUpdate, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("feed-consumer: subscribe %s: %w", mqx.TopicPostUpdate, err)
	}
	return c, nil
}

func consumeMessageBatch(ctx context.Context, svcCtx *svc.ServiceContext, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var e event.PostEvent
		if err := json.Unmarshal(msg.Body, &e); err != nil {
			logging.WithContext(ctx).Errorw("feed-consumer: unmarshal failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			feedConsumerMessages.Inc("invalid")
			continue
		}
		if err := e.Validate(); err != nil {
			logging.WithContext(ctx).Errorw("feed-consumer: invalid event, skipping",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			feedConsumerMessages.Inc("invalid")
			continue
		}
		if !visibilityx.IsPublished(int32(e.Status)) {
			// CORE-015：草稿、取消发布（status != 1）不进入关注流。
			feedConsumerMessages.Inc("skipped")
			continue
		}
		_, err := logic.HandlePostPublished(ctx,
			svcCtx.OutboxModel, svcCtx.InboxModel, svcCtx.UserService,
			svcCtx.BigVThreshold, svcCtx.FanoutBatchSize,
			logic.PostPublished{
				PostId: e.PostID, AuthorId: e.AuthorID, CreatedAt: e.EventTime,
			})
		if err != nil {
			logging.WithContext(ctx).Errorw("feed-consumer: fanout failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("post_id", e.PostID),
				logging.Field("err", err.Error()))
			feedConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		feedConsumerMessages.Inc("processed")
		observeFeedLag(e.EventTime, time.Now())
	}
	return consumer.ConsumeSuccess
}
