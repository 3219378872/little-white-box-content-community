package mqs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"esx/app/review/internal/intake"
	"esx/app/review/internal/store"
	"esx/app/review/worker/internal/svc"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"esx/pkg/logging"

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

// submissionIngester 是 consume 唯一依赖的能力；生产使用 *intake.Ingester。
type submissionIngester interface {
	Ingest(ctx context.Context, sub event.ReviewSubmittedEvent) (store.IngestResult, error)
}

var _ submissionIngester = (*intake.Ingester)(nil)

// consume 逐条接入送审消息；无效载荷跳过，其他失败整批重试。接入按送审唯一键幂等，重放不会重复建任务。
func consume(ctx context.Context, in submissionIngester, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		sub, err := intake.Decode(msg.Body)
		if err == nil {
			_, err = in.Ingest(ctx, sub)
		}
		var invalid intake.ErrInvalidSubmission
		if errors.As(err, &invalid) {
			logging.WithContext(ctx).Errorw("review-worker: invalid submission skipped",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			continue
		}
		if err != nil {
			logging.WithContext(ctx).Errorw("review-worker: ingest failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
