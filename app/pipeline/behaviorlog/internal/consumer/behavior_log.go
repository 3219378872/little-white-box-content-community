package consumer

import (
	"context"
	"encoding/json"
	"time"

	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"util"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type Deduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
}

func consumeBehaviorMsg(ctx context.Context, s store.BehaviorStore, d Deduper, msg *primitive.MessageExt) consumer.ConsumeResult {
	var e event.BehaviorEvent
	if err := json.Unmarshal(msg.Body, &e); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: unmarshal failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
		return consumer.ConsumeSuccess
	}

	if e.EventID == 0 {
		id, err := util.NextID()
		if err != nil {
			logx.WithContext(ctx).Errorw("behavior-log: generate event_id failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		e.EventID = id
	}

	if e.EventTime == 0 {
		e.EventTime = time.Now().UnixMilli()
	}

	if err := e.Validate(); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: validation failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
		return consumer.ConsumeSuccess
	}

	dup, err := d.IsDuplicate(ctx, e.EventIDString())
	if err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: dedup check failed, falling through",
			logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
	} else if dup {
		logx.WithContext(ctx).Infow("behavior-log: duplicate event skipped",
			logx.Field("event_id", e.EventID))
		return consumer.ConsumeSuccess
	}

	if err := s.Insert(ctx, e); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: insert failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("event_id", e.EventID),
			logx.Field("err", err.Error()))
		return consumer.ConsumeRetryLater
	}

	logx.WithContext(ctx).Infow("behavior-log: event recorded",
		logx.Field("event_id", e.EventID), logx.Field("user_id", e.UserID),
		logx.Field("action", e.Action))

	return consumer.ConsumeSuccess
}

func MakeBehaviorHandler(s store.BehaviorStore, d Deduper) func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
	return func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			result := consumeBehaviorMsg(ctx, s, d, msg)
			if result == consumer.ConsumeRetryLater {
				return consumer.ConsumeRetryLater, nil
			}
		}
		return consumer.ConsumeSuccess, nil
	}
}
