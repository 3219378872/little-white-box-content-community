package behavior

import (
	"context"
	"testing"

	"errx"
	"esx/app/behavior/rpc/behaviorservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"google.golang.org/grpc"
)

type fakeBehaviorService struct {
	behaviorservice.BehaviorService
	recordEventsFn func(context.Context, *behaviorservice.RecordEventsReq, ...grpc.CallOption) (*behaviorservice.RecordEventsResp, error)
}

func (f *fakeBehaviorService) RecordEvents(ctx context.Context, in *behaviorservice.RecordEventsReq, opts ...grpc.CallOption) (*behaviorservice.RecordEventsResp, error) {
	return f.recordEventsFn(ctx, in, opts...)
}

func validBehaviorEvent(id string) types.BehaviorEvent {
	position := int32(3)
	duration := int64(1200)
	return types.BehaviorEvent{
		ClientEventId: id,
		OccurredAt:    1720000000000,
		Action:        "play",
		TargetId:      123,
		TargetType:    "post",
		Scene:         "home",
		RequestId:     "recommend-request",
		Position:      &position,
		DurationMs:    &duration,
		RecallSource:  "itemcf",
		ModelVersion:  "rank-v1",
		ExperimentId:  "exp-home-v1",
	}
}

func TestRecordBehaviorEvents_IdentityAndPartialSuccess(t *testing.T) {
	tests := []struct {
		name            string
		ctx             context.Context
		anonymousID     string
		wantUserID      int64
		wantAnonymousID string
	}{
		{
			name:            "anonymous identity",
			ctx:             context.Background(),
			anonymousID:     "device-1",
			wantAnonymousID: "device-1",
		},
		{
			name:            "jwt identity",
			ctx:             jwtx.WithUserIdContext(context.Background(), 42),
			anonymousID:     "device-2",
			wantUserID:      42,
			wantAnonymousID: "device-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type requestContextKey struct{}
			ctxKey := requestContextKey{}
			ctx := context.WithValue(tt.ctx, ctxKey, "preserved")
			called := false
			svcCtx := &svc.ServiceContext{BehaviorService: &fakeBehaviorService{
				recordEventsFn: func(gotCtx context.Context, in *behaviorservice.RecordEventsReq, _ ...grpc.CallOption) (*behaviorservice.RecordEventsResp, error) {
					called = true
					if gotCtx.Value(ctxKey) != "preserved" {
						t.Fatal("request context was not propagated")
					}
					if in.UserId != tt.wantUserID || in.AnonymousId != tt.wantAnonymousID {
						t.Fatalf("unexpected identity: user=%d anonymous=%q", in.UserId, in.AnonymousId)
					}
					if in.SessionId != "session-1" || len(in.Events) != 2 {
						t.Fatalf("unexpected rpc request: %+v", in)
					}
					if in.Events[0].Position == nil || *in.Events[0].Position != 3 || in.Events[0].DurationMs == nil || *in.Events[0].DurationMs != 1200 {
						t.Fatalf("optional event fields were not preserved: %+v", in.Events[0])
					}
					return &behaviorservice.RecordEventsResp{
						Results: []*behaviorservice.RecordEventResult{
							{ClientEventId: "accepted", EventId: 1001, Accepted: true},
							{ClientEventId: "rejected", Accepted: false, Code: 2, Reason: "invalid event"},
						},
						AcceptedCount: 1,
						RejectedCount: 1,
					}, nil
				},
			}}

			logic := NewRecordBehaviorEventsLogic(ctx, svcCtx)
			resp, err := logic.RecordBehaviorEvents(&types.RecordBehaviorEventsReq{
				AnonymousId: tt.anonymousID,
				SessionId:   "session-1",
				Events: []types.BehaviorEvent{
					validBehaviorEvent("accepted"),
					validBehaviorEvent("rejected"),
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !called {
				t.Fatal("behavior rpc was not called")
			}
			if resp.AcceptedCount != 1 || resp.RejectedCount != 1 || len(resp.Results) != 2 {
				t.Fatalf("unexpected response: %+v", resp)
			}
			if !resp.Results[0].Accepted || resp.Results[0].EventId != 1001 {
				t.Fatalf("accepted result was not mapped: %+v", resp.Results[0])
			}
			if resp.Results[1].Accepted || resp.Results[1].Code != 2 || resp.Results[1].Reason != "invalid event" {
				t.Fatalf("rejected result was not mapped: %+v", resp.Results[1])
			}
		})
	}
}

func TestRecordBehaviorEvents_InvalidRequestDoesNotCallRPC(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		req  *types.RecordBehaviorEventsReq
	}{
		{name: "empty batch", ctx: context.Background(), req: &types.RecordBehaviorEventsReq{AnonymousId: "device-1"}},
		{name: "batch over limit", ctx: context.Background(), req: &types.RecordBehaviorEventsReq{AnonymousId: "device-1", Events: make([]types.BehaviorEvent, 101)}},
		{name: "anonymous id required", ctx: context.Background(), req: &types.RecordBehaviorEventsReq{Events: []types.BehaviorEvent{validBehaviorEvent("event-1")}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			svcCtx := &svc.ServiceContext{BehaviorService: &fakeBehaviorService{
				recordEventsFn: func(context.Context, *behaviorservice.RecordEventsReq, ...grpc.CallOption) (*behaviorservice.RecordEventsResp, error) {
					called = true
					return &behaviorservice.RecordEventsResp{}, nil
				},
			}}
			_, err := NewRecordBehaviorEventsLogic(tt.ctx, svcCtx).RecordBehaviorEvents(tt.req)
			if !errx.Is(err, errx.ParamError) {
				t.Fatalf("expected ParamError, got %v", err)
			}
			if called {
				t.Fatal("rpc must not be called for invalid input")
			}
		})
	}
}

func TestRecordBehaviorEvents_RPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{BehaviorService: &fakeBehaviorService{
		recordEventsFn: func(context.Context, *behaviorservice.RecordEventsReq, ...grpc.CallOption) (*behaviorservice.RecordEventsResp, error) {
			return nil, context.DeadlineExceeded
		},
	}}

	_, err := NewRecordBehaviorEventsLogic(context.Background(), svcCtx).RecordBehaviorEvents(&types.RecordBehaviorEventsReq{
		AnonymousId: "device-1",
		Events:      []types.BehaviorEvent{validBehaviorEvent("event-1")},
	})
	if err == nil {
		t.Fatal("expected rpc error")
	}
}
