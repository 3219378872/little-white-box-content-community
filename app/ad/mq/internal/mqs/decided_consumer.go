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

	"esx/pkg/logging"

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

// consume 逐条应用审核结论；无效消息跳过，应用失败整批重试。结论按 revision 条件更新，重放不会回退状态。
func consume(ctx context.Context, applier decisionApplier, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var decided event.ReviewDecidedEvent
		if err := json.Unmarshal(msg.Body, &decided); err != nil || decided.Validate() != nil {
			logging.WithContext(ctx).Errorw("ad-mq: invalid review decision skipped", logging.Field("msg_id", msg.MsgId))
			continue
		}
		if err := applier.Apply(ctx, decided); err != nil {
			logging.WithContext(ctx).Errorw("ad-mq: apply decision failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("objectId", decided.ObjectID), logging.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
