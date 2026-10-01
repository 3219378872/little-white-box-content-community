package mqs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"esx/app/review/internal/intake"
	"esx/app/review/worker/internal/svc"
	"esx/pkg/mqx"

	logx "esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// NewSubmittedConsumer 订阅 review-submitted 的全部业务类型，按唯一键幂等建任务（RVW-001）。
func NewSubmittedConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("review-worker: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consume(ctx, svcCtx.Ingester, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicReviewSubmitted, "*", handler); err != nil {
		return nil, fmt.Errorf("review-worker: subscribe: %w", err)
	}
	return c, nil
}

func consume(ctx context.Context, in *intake.Ingester, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		sub, err := intake.Decode(msg.Body)
		if err == nil {
			_, err = in.Ingest(ctx, sub)
		}
		var invalid intake.ErrInvalidSubmission
		if errors.As(err, &invalid) {
			logx.WithContext(ctx).Errorw("review-worker: invalid submission skipped",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if err != nil {
			logx.WithContext(ctx).Errorw("review-worker: ingest failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
