package logic

import (
	"context"
	"strings"
	"time"

	"esx/app/behavior/rpc/internal/publisher"
	"esx/app/behavior/rpc/internal/svc"
	pb "esx/kitex_gen/behavior"
	"esx/pkg/errx"
	"esx/pkg/event"

	"esx/pkg/logging"
	metric "esx/pkg/metrics"
)

// behaviorRecordTotal 按接受/拒绝统计单条事件。
var behaviorRecordTotal = metric.NewCounterVec(&metric.CounterVecOpts{
	Namespace: "esx", Subsystem: "behavior_rpc", Name: "record_events_total",
	Help: "Behavior events handled by outcome", Labels: []string{"outcome"},
})

// behaviorMQPublishTotal 统计投递到 MQ 的成功与失败次数。
var behaviorMQPublishTotal = metric.NewCounterVec(&metric.CounterVecOpts{
	Namespace: "esx", Subsystem: "behavior_rpc", Name: "mq_publish_total",
	Help: "Behavior event MQ publish attempts by outcome", Labels: []string{"outcome"},
})

// RecordEventsLogic 承载 RecordEvents 接口的业务逻辑；每个请求新建一个实例。
type RecordEventsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewRecordEventsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewRecordEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RecordEventsLogic {
	return &RecordEventsLogic{
		ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx),
	}
}

// RecordEvents 逐条校验并投递一批客户端行为事件；单条失败不影响同批其他事件，
// 结果按输入顺序逐条返回，整批只在请求本身非法时失败。
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

// recordOne 把客户端事件补全为服务端行为事件并投递到 MQ。EventID 由 client_event_id 确定性派生，
// 客户端重试同一事件会得到同一 ID，下游据此去重。
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
	// 拒绝明显过旧或来自未来的事件，避免客户端时钟错误污染统计窗口。
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
			logging.Field("client_event_id", input.ClientEventId), logging.Field("err", err.Error()))
		behaviorMQPublishTotal.Inc("failure")
		return rejected(input.ClientEventId, behavior.EventID, errx.ServiceUnavailable, "event publish failed")
	}
	behaviorMQPublishTotal.Inc("success")
	return &pb.RecordEventResult{
		ClientEventId: input.ClientEventId, EventId: behavior.EventID,
		Accepted: true, Code: errx.SUCCESS,
	}
}

// rejected 构造一条被拒绝的事件结果。
func rejected(clientEventID string, eventID int64, code int, reason string) *pb.RecordEventResult {
	return &pb.RecordEventResult{
		ClientEventId: clientEventID, EventId: eventID, Accepted: false,
		Code: int32(code), Reason: reason,
	}
}

// cloneInt32 复制可选字段，避免事件与请求共享指针。
func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// cloneInt64 复制可选字段，避免事件与请求共享指针。
func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
