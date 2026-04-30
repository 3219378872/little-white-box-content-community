package behaviorlog

import (
	"context"
	"encoding/json"

	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	"esx/pkg/event"
	"mqx"

	rocketconsumer "github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

func consumeBehaviorMsg(ctx context.Context, processor behaviorlogic.BehaviorProcessor, msg *primitive.MessageExt) rocketconsumer.ConsumeResult {
	e, err := parseBehaviorEvent(msg)
	if err != nil {
		logPermanent(ctx, msg, err)
		return rocketconsumer.ConsumeSuccess
	}

	if err := processor.Process(ctx, e, metaFromMessage(msg)); err != nil {
		if mqx.IsPermanentEvent(err) {
			logPermanent(ctx, msg, err)
			return rocketconsumer.ConsumeSuccess
		}
		logx.WithContext(ctx).Errorw("behavior-log: process failed",
			logx.Field("msg_id", msg.MsgId), logx.Field("event_id", e.EventID),
			logx.Field("err", err.Error()))
		return rocketconsumer.ConsumeRetryLater
	}

	return rocketconsumer.ConsumeSuccess
}

func MakeBehaviorHandler(processor behaviorlogic.BehaviorProcessor) func(ctx context.Context, msgs ...*primitive.MessageExt) (rocketconsumer.ConsumeResult, error) {
	return func(ctx context.Context, msgs ...*primitive.MessageExt) (rocketconsumer.ConsumeResult, error) {
		for _, msg := range msgs {
			result := consumeBehaviorMsg(ctx, processor, msg)
			if result == rocketconsumer.ConsumeRetryLater {
				return rocketconsumer.ConsumeRetryLater, nil
			}
		}
		return rocketconsumer.ConsumeSuccess, nil
	}
}

func parseBehaviorEvent(msg *primitive.MessageExt) (event.BehaviorEvent, error) {
	var e event.BehaviorEvent
	if err := json.Unmarshal(msg.Body, &e); err != nil {
		return event.BehaviorEvent{}, mqx.ErrPermanentEvent("unmarshal behavior event: " + err.Error())
	}
	return e, nil
}

func logPermanent(ctx context.Context, msg *primitive.MessageExt, err error) {
	logx.WithContext(ctx).Errorw("behavior-log: permanent event skipped",
		logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
}

func metaFromMessage(msg *primitive.MessageExt) behaviorlogic.MessageMeta {
	return behaviorlogic.MessageMeta{
		MsgID:          msg.MsgId,
		OffsetMsgID:    msg.OffsetMsgId,
		StoreTimestamp: msg.StoreTimestamp,
		BornTimestamp:  msg.BornTimestamp,
	}
}
