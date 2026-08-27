package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/assistant/mq/internal/svc"
	"esx/app/assistant/watch"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

func NewWatchConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("watch-matcher: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeWatchBatch(ctx, svcCtx.Watch, msgs...), nil
	}
	for _, topic := range []string{mqx.TopicPostCreate, mqx.TopicPostUpdate, mqx.TopicPostDelete} {
		if err := c.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			return nil, fmt.Errorf("watch-matcher: subscribe %s: %w", topic, err)
		}
	}
	return c, nil
}

func consumeWatchBatch(ctx context.Context, store watch.Store, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var ev event.PostEvent
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			logx.WithContext(ctx).Errorw("watch-matcher: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			watchConsumerMessages.Inc("invalid")
			continue
		}
		if err := watch.ApplyPostEvent(ctx, store, ev); err != nil {
			logx.WithContext(ctx).Errorw("watch-matcher: apply failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", ev.PostID),
				logx.Field("err", err.Error()))
			watchConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		logx.WithContext(ctx).Infow("watch-matcher: event applied",
			logx.Field("post_id", ev.PostID), logx.Field("type", string(ev.Type)))
		watchConsumerMessages.Inc("processed")
		observeWatchLag(ev.EventTime, time.Now())
	}
	return consumer.ConsumeSuccess
}
