package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/content/mq/cleanup/internal/store"
	"esx/app/content/mq/cleanup/internal/svc"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// countSyncTagExpression 订阅互动权威事件：like/unlike/favorite/unfavorite。
const countSyncTagExpression = "like || unlike || favorite || unfavorite"

// NewCountSyncConsumer 订阅 user-behavior-v2 上的互动动作，把计数同步到内容表。
// CORE-032：公开计数允许最终一致，但必须在 30 秒内收敛；outbox 同事务投递。
func NewCountSyncConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	cfg, err := svcCtx.Config.CountSyncConsumerConfig()
	if err != nil {
		return nil, err
	}
	c, err := mqx.NewConsumer(cfg)
	if err != nil {
		return nil, fmt.Errorf("count-sync-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeCountSyncBatch(ctx, svcCtx.CountSyncStore, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicUserBehaviorV2, countSyncTagExpression, handler); err != nil {
		return nil, fmt.Errorf("count-sync-consumer: subscribe %s: %w", mqx.TopicUserBehaviorV2, err)
	}
	return c, nil
}

func consumeCountSyncBatch(ctx context.Context, cs store.CountSyncStore, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var behavior event.BehaviorEvent
		if err := json.Unmarshal(msg.Body, &behavior); err != nil {
			logging.WithContext(ctx).Errorw("count-sync-consumer: unmarshal failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("invalid")
			// 结构化损坏的事件无法恢复，转入死信语义由 MQ 重试上限兜底。
			continue
		}
		if err := behavior.Validate(); err != nil {
			logging.WithContext(ctx).Errorw("count-sync-consumer: invalid event, skipping",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("invalid")
			continue
		}
		if err := cs.ApplyBehaviorCount(ctx, behavior); err != nil {
			logging.WithContext(ctx).Errorw("count-sync-consumer: apply count failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("event_id", behavior.EventID),
				logging.Field("action", behavior.Action), logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		countSyncConsumerMessages.Inc("processed")
		observeCountSyncLag(behavior.EventTime, time.Now())
		logging.WithContext(ctx).Infow("count-sync-consumer: count applied",
			logging.Field("event_id", behavior.EventID),
			logging.Field("action", behavior.Action),
			logging.Field("target_id", behavior.TargetID),
			logging.Field("target_type", behavior.TargetType))
	}
	return consumer.ConsumeSuccess
}
