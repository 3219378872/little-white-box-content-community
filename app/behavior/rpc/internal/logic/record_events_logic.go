package logic

import (
	"context"
	"strings"
	"time"

	"esx/app/behavior/rpc/internal/publisher"
	"esx/app/behavior/rpc/internal/svc"
	"esx/app/behavior/rpc/xiaobaihe/behavior/pb"
	"esx/pkg/errx"
	"esx/pkg/event"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/metric"
)

var behaviorRecordTotal = metric.NewCounterVec(&metric.CounterVecOpts{
	Namespace: "esx", Subsystem: "behavior_rpc", Name: "record_events_total",
	Help: "Behavior events handled by outcome", Labels: []string{"outcome"},
})

var behaviorMQPublishTotal = metric.NewCounterVec(&metric.CounterVecOpts{
	Namespace: "esx", Subsystem: "behavior_rpc", Name: "mq_publish_total",
	Help: "Behavior event MQ publish attempts by outcome", Labels: []string{"outcome"},
})

type RecordEventsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRecordEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RecordEventsLogic {
	return &RecordEventsLogic{
		ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx),
	}
}

func (l *RecordEventsLogic) RecordEvents(in *pb.RecordEventsReq) (*pb.RecordEventsResp, error) {
	if in == nil || len(in.Events) == 0 || len(in.Events) > l.svcCtx.Config.MaxBatchSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.UserId <= 0 && strings.TrimSpace(in.AnonymousId) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	now := time.Now()
	if l.svcCtx.Now != nil {
		now = l.svcCtx.Now()
	}
	receivedAt := now.UnixMilli()
	response := &pb.RecordEventsResp{Results: make([]*pb.RecordEventResult, 0, len(in.Events))}
	for _, input := range in.Events {
		result := l.recordOne(in, input, now, receivedAt)
		response.Results = append(response.Results, result)
		if result.Accepted {
			response.AcceptedCount++
			behaviorRecordTotal.Inc("accepted")
		} else {
			response.RejectedCount++
			behaviorRecordTotal.Inc("rejected")
		}
	}
	return response, nil
}

func (l *RecordEventsLogic) recordOne(
	request *pb.RecordEventsReq,
	input *pb.ClientBehaviorEvent,
	now time.Time,
	receivedAt int64,
) *pb.RecordEventResult {
	if input == nil {
		return rejected("", 0, errx.ParamError, "event is required")
	}
	behavior := event.BehaviorEvent{
		EventID:       event.DeterministicBehaviorEventID(input.ClientEventId),
		ClientEventID: input.ClientEventId,
		SchemaVersion: event.BehaviorSchemaVersion,
		EventTime:     input.OccurredAt,
		ReceivedAt:    receivedAt,
		UserID:        request.UserId,
		AnonymousID:   request.AnonymousId,
		SessionID:     request.SessionId,
		RequestID:     input.RequestId,
		Action:        input.Action,
		TargetID:      input.TargetId,
		TargetType:    input.TargetType,
		Scene:         input.Scene,
		Position:      cloneInt32(input.Position),
		DurationMs:    cloneInt64(input.DurationMs),
		RecallSource:  input.RecallSource,
		ModelVersion:  input.ModelVersion,
		ExperimentID:  input.ExperimentId,
		Producer:      "behavior-rpc",
		ClientIP:      request.ClientIp,
		ClientVersion: request.ClientVersion,
	}
	if err := behavior.ValidateClientSubmitted(); err != nil {
		return rejected(input.ClientEventId, behavior.EventID, errx.ParamError, err.Error())
	}
	eventTime := time.UnixMilli(behavior.EventTime)
	oldest := now.Add(-time.Duration(l.svcCtx.Config.MaxPastAgeHours) * time.Hour)
	latest := now.Add(time.Duration(l.svcCtx.Config.MaxFutureSkewSeconds) * time.Second)
	if eventTime.Before(oldest) || eventTime.After(latest) {
		return rejected(input.ClientEventId, behavior.EventID, errx.ParamError, "occurred_at is outside the accepted clock window")
	}
	if l.svcCtx.Publisher == nil {
		l.Errorw("behavior publisher is not configured")
		behaviorMQPublishTotal.Inc("failure")
		return rejected(input.ClientEventId, behavior.EventID, errx.ServiceUnavailable, "event publish failed")
	}
	if err := l.svcCtx.Publisher.Publish(l.ctx, behavior, publisher.Metadata{
		TraceID: request.TraceId, UserAgent: request.UserAgent,
	}); err != nil {
		l.Errorw("publish behavior event failed",
			logx.Field("client_event_id", input.ClientEventId), logx.Field("err", err.Error()))
		behaviorMQPublishTotal.Inc("failure")
		return rejected(input.ClientEventId, behavior.EventID, errx.ServiceUnavailable, "event publish failed")
	}
	behaviorMQPublishTotal.Inc("success")
	return &pb.RecordEventResult{
		ClientEventId: input.ClientEventId, EventId: behavior.EventID,
		Accepted: true, Code: errx.SUCCESS,
	}
}

func rejected(clientEventID string, eventID int64, code int, reason string) *pb.RecordEventResult {
	return &pb.RecordEventResult{
		ClientEventId: clientEventID, EventId: eventID, Accepted: false,
		Code: int32(code), Reason: reason,
	}
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
