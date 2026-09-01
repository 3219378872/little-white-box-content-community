package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"esx/app/assistant/mq/internal/svc"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type matcher struct {
	Store                watch.Store
	SpikeMinComments     int
	SpikeJudge           watch.SpikeJudge
	ValidatePost         func(context.Context, event.PostEvent) (bool, error)
	ValidateBehaviorPost func(context.Context, int64) (bool, error)
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
		m.ValidatePost = currentPostEventValidator(svcCtx.Content)
		m.ValidateBehaviorPost = currentPublishedPostValidator(svcCtx.Content)
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
		if msg == nil {
			watchConsumerMessages.Inc("invalid")
			continue
		}
		var ev event.PostEvent
		if err := json.Unmarshal(msg.Body, &ev); err != nil {
			logx.WithContext(ctx).Errorw("watch-matcher: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			watchConsumerMessages.Inc("invalid")
			continue
		}
		if err := ev.Validate(); err != nil {
			watchConsumerMessages.Inc("invalid")
			continue
		}
		if m.ValidatePost != nil {
			current, err := m.ValidatePost(ctx, ev)
			if err != nil {
				logx.WithContext(ctx).Errorw("watch-matcher: current post check failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", ev.PostID),
					logx.Field("err", err.Error()))
				watchConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
			if !current {
				watchConsumerMessages.Inc("skipped")
				continue
			}
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

func currentPostEventValidator(content contentservice.ContentService) func(context.Context, event.PostEvent) (bool, error) {
	return func(ctx context.Context, ev event.PostEvent) (bool, error) {
		if ev.Type == event.PostEventDeleted || ev.Status != 1 {
			return false, nil
		}
		if content == nil {
			return false, errx.NewWithCode(errx.ServiceUnavailable)
		}
		resp, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: ev.PostID})
		if err != nil {
			converted := errx.FromRPCError(err)
			if errx.Is(converted, errx.ContentNotFound) {
				return false, nil
			}
			return false, converted
		}
		if resp == nil || resp.Post == nil || resp.Post.Status != 1 {
			return false, nil
		}
		return resp.Post.Revision == ev.Revision, nil
	}
}

func consumeBehaviorMessage(ctx context.Context, m matcher, msg *primitive.MessageExt) consumer.ConsumeResult {
	var ev event.BehaviorEvent
	if err := json.Unmarshal(msg.Body, &ev); err != nil {
		logx.WithContext(ctx).Errorw("watch-matcher: behavior unmarshal failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
		watchConsumerMessages.Inc("invalid")
		return consumer.ConsumeSuccess
	}
	if ev.Action == event.BehaviorActionComment && ev.TargetID > 0 &&
		strings.EqualFold(strings.TrimSpace(ev.TargetType), "post") && m.ValidateBehaviorPost != nil {
		current, err := m.ValidateBehaviorPost(ctx, ev.TargetID)
		if err != nil {
			logx.WithContext(ctx).Errorw("watch-matcher: behavior post check failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", ev.TargetID), logx.Field("err", err.Error()))
			watchConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		if !current {
			watchConsumerMessages.Inc("skipped")
			return consumer.ConsumeSuccess
		}
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

func currentPublishedPostValidator(content contentservice.ContentService) func(context.Context, int64) (bool, error) {
	return func(ctx context.Context, postID int64) (bool, error) {
		if content == nil {
			return false, errx.NewWithCode(errx.ServiceUnavailable)
		}
		resp, err := content.GetPost(ctx, &contentservice.GetPostReq{PostId: postID})
		if err != nil {
			converted := errx.FromRPCError(err)
			if errx.Is(converted, errx.ContentNotFound) {
				return false, nil
			}
			return false, converted
		}
		return resp != nil && resp.Post != nil && resp.Post.Status == 1, nil
	}
}
