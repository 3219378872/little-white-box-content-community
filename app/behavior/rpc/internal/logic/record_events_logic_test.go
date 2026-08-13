package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"errx"
	"esx/app/behavior/rpc/internal/config"
	"esx/app/behavior/rpc/internal/publisher"
	"esx/app/behavior/rpc/internal/svc"
	"esx/app/behavior/rpc/xiaobaihe/behavior/pb"
	"esx/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePublisher struct {
	events   []event.BehaviorEvent
	metadata []publisher.Metadata
	ctx      context.Context
	failID   string
}

func (p *fakePublisher) Publish(ctx context.Context, behavior event.BehaviorEvent, metadata publisher.Metadata) error {
	p.ctx = ctx
	if behavior.ClientEventID == p.failID {
		return errors.New("broker unavailable")
	}
	p.events = append(p.events, behavior)
	p.metadata = append(p.metadata, metadata)
	return nil
}

func behaviorTestContext(p publisher.Publisher) *svc.ServiceContext {
	now := time.UnixMilli(1720000000000)
	return &svc.ServiceContext{
		Config:    config.Config{MaxBatchSize: 100, MaxPastAgeHours: 720, MaxFutureSkewSeconds: 300},
		Publisher: p,
		Now:       func() time.Time { return now },
	}
}

func exposure(clientID string) *pb.ClientBehaviorEvent {
	position := int32(3)
	return &pb.ClientBehaviorEvent{
		ClientEventId: clientID, OccurredAt: 1720000000000,
		Action: event.BehaviorActionExposure, TargetId: 123, TargetType: "post",
		Scene: "home", RequestId: "request-1", Position: &position,
	}
}

func TestRecordEventsAcceptsAuthenticatedBatch(t *testing.T) {
	p := &fakePublisher{}
	type requestContextKey struct{}
	ctx := context.WithValue(context.Background(), requestContextKey{}, "request-context")
	logic := NewRecordEventsLogic(ctx, behaviorTestContext(p))

	resp, err := logic.RecordEvents(&pb.RecordEventsReq{
		UserId: 42, SessionId: "session-1", ClientIp: "10.0.0.1",
		ClientVersion: "2.0.0", TraceId: "trace-1", UserAgent: "test-agent",
		Events: []*pb.ClientBehaviorEvent{exposure("client-1")},
	})

	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, int32(1), resp.AcceptedCount)
	assert.True(t, resp.Results[0].Accepted)
	require.Len(t, p.events, 1)
	assert.Equal(t, ctx, p.ctx)
	assert.Equal(t, int64(42), p.events[0].UserID)
	assert.Equal(t, event.BehaviorSchemaVersion, p.events[0].SchemaVersion)
	assert.Equal(t, "behavior-rpc", p.events[0].Producer)
	assert.Equal(t, "trace-1", p.metadata[0].TraceID)
}

func TestRecordEventsAllowsAnonymousIdentity(t *testing.T) {
	p := &fakePublisher{}
	logic := NewRecordEventsLogic(context.Background(), behaviorTestContext(p))

	resp, err := logic.RecordEvents(&pb.RecordEventsReq{
		AnonymousId: "device-1", Events: []*pb.ClientBehaviorEvent{exposure("client-1")},
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.AcceptedCount)
	assert.Equal(t, "device-1", p.events[0].AnonymousID)
}

func TestRecordEventsReturnsPartialResults(t *testing.T) {
	p := &fakePublisher{failID: "publish-fails"}
	logic := NewRecordEventsLogic(context.Background(), behaviorTestContext(p))
	invalid := exposure("invalid")
	invalid.RequestId = ""

	resp, err := logic.RecordEvents(&pb.RecordEventsReq{
		UserId: 42,
		Events: []*pb.ClientBehaviorEvent{
			exposure("accepted"), invalid, exposure("publish-fails"),
		},
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.AcceptedCount)
	assert.Equal(t, int32(2), resp.RejectedCount)
	assert.Equal(t, int32(errx.ParamError), resp.Results[1].Code)
	assert.Contains(t, resp.Results[1].Reason, "request_id")
	assert.Equal(t, int32(errx.ServiceUnavailable), resp.Results[2].Code)
	assert.Equal(t, "event publish failed", resp.Results[2].Reason)
}

func TestRecordEventsUsesStableEventIDOnRetry(t *testing.T) {
	p := &fakePublisher{}
	logic := NewRecordEventsLogic(context.Background(), behaviorTestContext(p))
	request := &pb.RecordEventsReq{UserId: 42, Events: []*pb.ClientBehaviorEvent{exposure("stable-client-id")}}

	first, err := logic.RecordEvents(request)
	require.NoError(t, err)
	second, err := logic.RecordEvents(request)
	require.NoError(t, err)
	assert.Equal(t, first.Results[0].EventId, second.Results[0].EventId)
	assert.Equal(t, event.DeterministicBehaviorEventID("stable-client-id"), first.Results[0].EventId)
}

func TestRecordEventsRejectsInvalidRequestShape(t *testing.T) {
	logic := NewRecordEventsLogic(context.Background(), behaviorTestContext(&fakePublisher{}))

	_, err := logic.RecordEvents(&pb.RecordEventsReq{Events: []*pb.ClientBehaviorEvent{exposure("client-1")}})
	assert.True(t, errx.Is(err, errx.ParamError))

	tooMany := make([]*pb.ClientBehaviorEvent, 101)
	_, err = logic.RecordEvents(&pb.RecordEventsReq{UserId: 1, Events: tooMany})
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestRecordEventsRejectsClockSkewWithoutPublishing(t *testing.T) {
	p := &fakePublisher{}
	logic := NewRecordEventsLogic(context.Background(), behaviorTestContext(p))

	t.Run("超过30天回补窗口拒绝", func(t *testing.T) {
		stale := exposure("stale")
		stale.OccurredAt = time.UnixMilli(1720000000000).Add(-31 * 24 * time.Hour).UnixMilli()

		resp, err := logic.RecordEvents(&pb.RecordEventsReq{UserId: 42, Events: []*pb.ClientBehaviorEvent{stale}})

		require.NoError(t, err)
		assert.Equal(t, int32(1), resp.RejectedCount)
		assert.Empty(t, p.events)
	})

	t.Run("超过5分钟超前窗口拒绝", func(t *testing.T) {
		future := exposure("future")
		future.OccurredAt = time.UnixMilli(1720000000000).Add(6 * time.Minute).UnixMilli()

		resp, err := logic.RecordEvents(&pb.RecordEventsReq{UserId: 42, Events: []*pb.ClientBehaviorEvent{future}})

		require.NoError(t, err)
		assert.Equal(t, int32(1), resp.RejectedCount)
		assert.Empty(t, p.events)
	})
}

func TestRecordEventsRejectsAuthoritativeActionsFromClients(t *testing.T) {
	p := &fakePublisher{}
	logic := NewRecordEventsLogic(context.Background(), behaviorTestContext(p))

	for _, action := range []string{
		event.BehaviorActionLike, event.BehaviorActionFollow, event.BehaviorActionComment,
		event.BehaviorActionFavorite, event.BehaviorActionUnlike,
	} {
		e := exposure("client-" + action)
		e.Action = action
		resp, err := logic.RecordEvents(&pb.RecordEventsReq{UserId: 42, Events: []*pb.ClientBehaviorEvent{e}})
		require.NoError(t, err)
		require.Len(t, resp.Results, 1)
		assert.False(t, resp.Results[0].Accepted, "action %s must be rejected from clients", action)
		assert.Equal(t, int32(errx.ParamError), resp.Results[0].Code)
		assert.Contains(t, resp.Results[0].Reason, "not allowed from clients")
	}
	assert.Empty(t, p.events)
}
