package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/recommend/mq/internal/store"
	"esx/app/recommend/mq/internal/svc"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

func NewRecommendConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("recommend-consumer: create consumer: %w", err)
	}
	behaviorHandler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeBehaviorBatch(ctx, svcCtx.BehaviorStore, svcCtx.DeadLetters, msgs...), nil
	}
	if err := c.SubscribeWithTopic(svcCtx.Config.MQ.Topic, svcCtx.Config.MQ.Tag, behaviorHandler); err != nil {
		return nil, fmt.Errorf("recommend-consumer: subscribe %s: %w", svcCtx.Config.MQ.Topic, err)
	}
	postHandler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumePostBatch(ctx, svcCtx.CandidateStore, svcCtx.DeadLetters, msgs...), nil
	}
	for _, topic := range []string{mqx.TopicPostCreate, mqx.TopicPostUpdate, mqx.TopicPostDelete} {
		if err := c.SubscribeWithTopic(topic, mqx.TagDefault, postHandler); err != nil {
			return nil, fmt.Errorf("recommend-consumer: subscribe %s: %w", topic, err)
		}
	}
	return c, nil
}

func consumeBehaviorBatch(
	ctx context.Context,
	bs store.BehaviorStore,
	deadLetters store.DeadLetterRecorder,
	msgs ...*primitive.MessageExt,
) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var behavior event.BehaviorEvent
		if err := json.Unmarshal(msg.Body, &behavior); err != nil {
			logging.WithContext(ctx).Errorw("recommend-consumer: unmarshal failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			if recordDeadLetter(ctx, deadLetters, msg, err) != nil {
				recommendConsumerMessages.Inc("behavior", "retry")
				return consumer.ConsumeRetryLater
			}
			recommendConsumerMessages.Inc("behavior", "dead_letter")
			continue
		}
		if err := behavior.Validate(); err != nil {
			logging.WithContext(ctx).Errorw("recommend-consumer: invalid behavior event",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			if recordDeadLetter(ctx, deadLetters, msg, err) != nil {
				recommendConsumerMessages.Inc("behavior", "retry")
				return consumer.ConsumeRetryLater
			}
			recommendConsumerMessages.Inc("behavior", "dead_letter")
			continue
		}
		// ADS-027：广告事件不进入帖子推荐特征、训练数据与帖子曝光去重。
		if behavior.TargetType == event.BehaviorTargetAd {
			recommendConsumerMessages.Inc("behavior", "skipped_ad")
			continue
		}
		if err := bs.Record(ctx, behavior); err != nil {
			logging.WithContext(ctx).Errorw("recommend-consumer: record behavior failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("event_id", behavior.EventID),
				logging.Field("err", err.Error()))
			recommendConsumerMessages.Inc("behavior", "retry")
			return consumer.ConsumeRetryLater
		}
		recommendConsumerMessages.Inc("behavior", "processed")
		observeRecommendEventLag("behavior", behavior.EventTime, time.Now())
		logging.WithContext(ctx).Infow("recommend-consumer: behavior recorded",
			logging.Field("event_id", behavior.EventID), logging.Field("action", behavior.Action))
	}
	return consumer.ConsumeSuccess
}

func consumePostBatch(
	ctx context.Context,
	candidates store.CandidateStore,
	deadLetters store.DeadLetterRecorder,
	msgs ...*primitive.MessageExt,
) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var post event.PostEvent
		if err := json.Unmarshal(msg.Body, &post); err != nil {
			if recordDeadLetter(ctx, deadLetters, msg, err) != nil {
				recommendConsumerMessages.Inc("post", "retry")
				return consumer.ConsumeRetryLater
			}
			recommendConsumerMessages.Inc("post", "dead_letter")
			continue
		}
		if err := post.Validate(); err != nil {
			if recordDeadLetter(ctx, deadLetters, msg, err) != nil {
				recommendConsumerMessages.Inc("post", "retry")
				return consumer.ConsumeRetryLater
			}
			recommendConsumerMessages.Inc("post", "dead_letter")
			continue
		}
		if err := candidates.RecordPost(ctx, post); err != nil {
			logging.WithContext(ctx).Errorw("recommend-consumer: record post candidate failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("post_id", post.PostID),
				logging.Field("err", err.Error()))
			recommendConsumerMessages.Inc("post", "retry")
			return consumer.ConsumeRetryLater
		}
		recommendConsumerMessages.Inc("post", "processed")
		observeRecommendEventLag("post", post.EventTime, time.Now())
	}
	return consumer.ConsumeSuccess
}

func recordDeadLetter(
	ctx context.Context,
	recorder store.DeadLetterRecorder,
	msg *primitive.MessageExt,
	cause error,
) error {
	if recorder == nil {
		return fmt.Errorf("recommend-consumer: dead letter recorder is not configured")
	}
	if err := recorder.RecordDeadLetter(ctx, msg.MsgId, msg.Body, cause); err != nil {
		logging.WithContext(ctx).Errorw("recommend-consumer: dead letter write failed",
			logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
		return err
	}
	return nil
}
