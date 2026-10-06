package behaviorlog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	behaviorlogic "esx/app/pipeline/behaviorlog/internal/logic"
	"esx/app/pipeline/behaviorlog/internal/store"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"esx/pkg/logging"

	rocketconsumer "github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

type DeadLetterRecorder interface {
	InsertDeadLetter(ctx context.Context, letter store.DeadLetter) error
}

func consumeBehaviorMsg(
	ctx context.Context,
	processor behaviorlogic.BehaviorProcessor,
	deadLetters DeadLetterRecorder,
	msg *primitive.MessageExt,
) rocketconsumer.ConsumeResult {
	e, err := parseBehaviorEvent(msg)
	if err != nil {
		if err := recordPermanent(ctx, deadLetters, msg, 0, err); err != nil {
			behaviorConsumerMessages.Inc("retry")
			return rocketconsumer.ConsumeRetryLater
		}
		behaviorConsumerMessages.Inc("dead_letter")
		return rocketconsumer.ConsumeSuccess
	}

	if err := processor.Process(ctx, e, metaFromMessage(msg)); err != nil {
		if mqx.IsPermanentEvent(err) {
			if err := recordPermanent(ctx, deadLetters, msg, e.EventID, err); err != nil {
				behaviorConsumerMessages.Inc("retry")
				return rocketconsumer.ConsumeRetryLater
			}
			behaviorConsumerMessages.Inc("dead_letter")
			return rocketconsumer.ConsumeSuccess
		}
		logging.WithContext(ctx).Errorw("behavior-log: process failed",
			logging.Field("msg_id", msg.MsgId), logging.Field("event_id", e.EventID),
			logging.Field("err", err.Error()))
		behaviorConsumerMessages.Inc("retry")
		return rocketconsumer.ConsumeRetryLater
	}

	behaviorConsumerMessages.Inc("processed")
	observeBehaviorEventLag(e.EventTime, time.Now())
	return rocketconsumer.ConsumeSuccess
}

func MakeBehaviorHandler(
	processor behaviorlogic.BehaviorProcessor,
	deadLetters DeadLetterRecorder,
) func(ctx context.Context, msgs ...*primitive.MessageExt) (rocketconsumer.ConsumeResult, error) {
	return func(ctx context.Context, msgs ...*primitive.MessageExt) (rocketconsumer.ConsumeResult, error) {
		for _, msg := range msgs {
			result := consumeBehaviorMsg(ctx, processor, deadLetters, msg)
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

func recordPermanent(ctx context.Context, deadLetters DeadLetterRecorder, msg *primitive.MessageExt, eventID int64, eventErr error) error {
	logging.WithContext(ctx).Errorw("behavior-log: permanent event skipped",
		logging.Field("msg_id", msg.MsgId), logging.Field("err", eventErr.Error()))
	if deadLetters == nil {
		return fmt.Errorf("behavior-log: dead letter store is not configured")
	}
	if err := deadLetters.InsertDeadLetter(ctx, store.DeadLetter{
		MessageID: msg.MsgId, EventID: eventID, Payload: msg.Body,
		Error: eventErr.Error(), ReceivedAt: msg.StoreTimestamp,
	}); err != nil {
		logging.WithContext(ctx).Errorw("behavior-log: dead letter insert failed",
			logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
		return err
	}
	return nil
}

func metaFromMessage(msg *primitive.MessageExt) behaviorlogic.MessageMeta {
	return behaviorlogic.MessageMeta{
		MsgID:          msg.MsgId,
		OffsetMsgID:    msg.OffsetMsgId,
		StoreTimestamp: msg.StoreTimestamp,
		BornTimestamp:  msg.BornTimestamp,
	}
}
