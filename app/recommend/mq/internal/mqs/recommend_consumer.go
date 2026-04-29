package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/recommend/mq/internal/store"
	"esx/app/recommend/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type behaviorEvent struct {
	UserID     int64  `json:"user_id"`
	Action     string `json:"action"`
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"`
}

func NewRecommendConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("recommend-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeBehaviorBatch(ctx, svcCtx.BehaviorStore, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicUserBehavior, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("recommend-consumer: subscribe %s: %w", mqx.TopicUserBehavior, err)
	}
	return c, nil
}

func consumeBehaviorBatch(ctx context.Context, bs store.BehaviorStore, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event behaviorEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("recommend-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.UserID <= 0 {
			logx.WithContext(ctx).Errorw("recommend-consumer: missing user_id",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		if event.Action == "" {
			logx.WithContext(ctx).Errorw("recommend-consumer: missing action",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		if err := bs.Record(ctx, store.BehaviorEvent{
			UserID: event.UserID, Action: event.Action,
			TargetID: event.TargetID, TargetType: event.TargetType,
		}); err != nil {
			logx.WithContext(ctx).Errorw("recommend-consumer: record behavior failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("user_id", event.UserID),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		logx.WithContext(ctx).Infow("recommend-consumer: behavior recorded",
			logx.Field("user_id", event.UserID), logx.Field("action", event.Action))
	}
	return consumer.ConsumeSuccess
}
