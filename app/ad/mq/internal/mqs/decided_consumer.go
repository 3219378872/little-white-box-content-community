package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/ad/mq/internal/apply"
	"esx/app/ad/mq/internal/svc"
	"esx/pkg/event"
	"esx/pkg/mqx"

	logx "esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// NewDecidedConsumer 按 tag 订阅广告相关的审核结论（DES-review-platform「结论下发与一致性」）。
func NewDecidedConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("ad-mq: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consume(ctx, svcCtx.Applier, msgs...), nil
	}
	tags := event.ReviewBizAdCreative + " || " + event.ReviewBizAdvertiserQualification
	if err := c.SubscribeWithTopic(mqx.TopicReviewDecided, tags, handler); err != nil {
		return nil, fmt.Errorf("ad-mq: subscribe: %w", err)
	}
	return c, nil
}

// decisionApplier 是 consume 唯一依赖的能力；生产使用 *apply.Applier。
type decisionApplier interface {
	Apply(ctx context.Context, d event.ReviewDecidedEvent) error
}

var _ decisionApplier = (*apply.Applier)(nil)

func consume(ctx context.Context, applier decisionApplier, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var decided event.ReviewDecidedEvent
		if err := json.Unmarshal(msg.Body, &decided); err != nil || decided.Validate() != nil {
			logx.WithContext(ctx).Errorw("ad-mq: invalid review decision skipped", logx.Field("msg_id", msg.MsgId))
			continue
		}
		if err := applier.Apply(ctx, decided); err != nil {
			logx.WithContext(ctx).Errorw("ad-mq: apply decision failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("objectId", decided.ObjectID), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
