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

type matcher struct {
	Store            watch.Store
	SpikeMinComments int
	SpikeJudge       watch.SpikeJudge
}

func NewWatchConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("watch-matcher: create consumer: %w", err)
	}
	m := matcherFromSvc(svcCtx)
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeWatchBatch(ctx, m, msgs...), nil
	}
	topics := []string{mqx.TopicPostCreate, mqx.TopicPostUpdate, mqx.TopicPostDelete, mqx.TopicUserBehaviorV2}
	for _, topic := range topics {
		if err := c.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			return nil, fmt.Errorf("watch-matcher: subscribe %s: %w", topic, err)
		}
	}
	return c, nil
}

func matcherFromSvc(svcCtx *svc.ServiceContext) matcher {
	m := matcher{SpikeMinComments: watch.DefaultSpikeMinComments}
	if svcCtx != nil {
		m.Store = svcCtx.Watch
		if svcCtx.Config.SpikeMinComments > 0 {
			m.SpikeMinComments = svcCtx.Config.SpikeMinComments
		}
	}
	return m
}

func consumeWatchBatch(ctx context.Context, m matcher, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		if msg != nil && msg.Topic == mqx.TopicUserBehaviorV2 {
			if result := consumeBehaviorMessage(ctx, m, msg); result == consumer.ConsumeRetryLater {
				return result
			}
			continue
		}
		var ev event.PostEvent
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			logx.WithContext(ctx).Errorw("watch-matcher: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			watchConsumerMessages.Inc("invalid")
			continue
		}
		if err := watch.ApplyPostEvent(ctx, m.Store, ev); err != nil {
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

func consumeBehaviorMessage(ctx context.Context, m matcher, msg *primitive.MessageExt) consumer.ConsumeResult {
	var ev event.BehaviorEvent
	if err := json.Unmarshal(msg.Body, &ev); err != nil {
		logx.WithContext(ctx).Errorw("watch-matcher: behavior unmarshal failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
		watchConsumerMessages.Inc("invalid")
		return consumer.ConsumeSuccess
	}
	if err := watch.ApplyBehaviorEvent(ctx, m.Store, ev, watch.SpikeOptions{
		MinComments: m.SpikeMinComments,
		Judge:       m.SpikeJudge,
	}); err != nil {
		logx.WithContext(ctx).Errorw("watch-matcher: behavior apply failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("target_id", ev.TargetID),
			logx.Field("err", err.Error()))
		watchConsumerMessages.Inc("retry")
		return consumer.ConsumeRetryLater
	}
	watchConsumerMessages.Inc("processed")
	observeWatchLag(ev.EventTime, time.Now())
	return consumer.ConsumeSuccess
}
