// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package behavior

import (
	"context"
	"errors"
	"strings"

	"errx"
	"esx/app/behavior/rpc/behaviorservice"
	"gateway/internal/middleware"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type RecordBehaviorEventsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 批量记录客户端行为
func NewRecordBehaviorEventsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RecordBehaviorEventsLogic {
	return &RecordBehaviorEventsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RecordBehaviorEventsLogic) RecordBehaviorEvents(req *types.RecordBehaviorEventsReq) (resp *types.RecordBehaviorEventsResp, err error) {
	if req == nil || len(req.Events) == 0 || len(req.Events) > 100 {
		return nil, errx.New(errx.ParamError, "行为事件数量必须在 1 到 100 之间")
	}

	userID, authenticated := jwtx.GetOptionalUserIdFromContext(l.ctx)
	if !authenticated || userID <= 0 {
		userID = 0
		if strings.TrimSpace(req.AnonymousId) == "" {
			return nil, errx.New(errx.ParamError, "匿名行为必须提供 anonymousId")
		}
	}

	events := make([]*behaviorservice.ClientBehaviorEvent, 0, len(req.Events))
	for _, event := range req.Events {
		events = append(events, &behaviorservice.ClientBehaviorEvent{
			ClientEventId: event.ClientEventId,
			OccurredAt:    event.OccurredAt,
			Action:        event.Action,
			TargetId:      event.TargetId,
			TargetType:    event.TargetType,
			Scene:         event.Scene,
			RequestId:     event.RequestId,
			Position:      event.Position,
			DurationMs:    event.DurationMs,
			RecallSource:  event.RecallSource,
			ModelVersion:  event.ModelVersion,
			ExperimentId:  event.ExperimentId,
		})
	}

	metadata := middleware.BehaviorRequestMetadataFromContext(l.ctx)
	result, err := l.svcCtx.BehaviorService.RecordEvents(l.ctx, &behaviorservice.RecordEventsReq{
		UserId:        userID,
		AnonymousId:   strings.TrimSpace(req.AnonymousId),
		SessionId:     strings.TrimSpace(req.SessionId),
		Events:        events,
		ClientIp:      metadata.ClientIP,
		UserAgent:     metadata.UserAgent,
		ClientVersion: metadata.ClientVersion,
		TraceId:       metadata.TraceID,
	})
	if err != nil {
		l.Errorw("BehaviorService.RecordEvents RPC failed", logx.Field("err", err.Error()))
		return nil, behaviorRPCError(err)
	}
	if result == nil {
		l.Error("BehaviorService.RecordEvents returned a nil response")
		return nil, errx.NewWithCode(errx.SystemError)
	}

	results := make([]types.BehaviorEventResult, 0, len(result.Results))
	for _, eventResult := range result.Results {
		if eventResult == nil {
			continue
		}
		results = append(results, types.BehaviorEventResult{
			ClientEventId: eventResult.ClientEventId,
			EventId:       eventResult.EventId,
			Accepted:      eventResult.Accepted,
			Code:          eventResult.Code,
			Reason:        eventResult.Reason,
		})
	}

	return &types.RecordBehaviorEventsResp{
		Results:       results,
		AcceptedCount: result.AcceptedCount,
		RejectedCount: result.RejectedCount,
	}, nil
}

func behaviorRPCError(err error) error {
	var bizErr *errx.BizError
	if errors.As(err, &bizErr) {
		return bizErr
	}
	return errx.Wrap(err, errx.SystemError)
}
