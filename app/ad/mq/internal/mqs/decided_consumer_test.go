package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/require"

	"esx/app/ad/mq/internal/svc"
	"esx/pkg/event"
)

type fakeApplier struct {
	applied []event.ReviewDecidedEvent
	err     error
}

func (f *fakeApplier) Apply(_ context.Context, d event.ReviewDecidedEvent) error {
	f.applied = append(f.applied, d)
	return f.err
}

func decidedMessage(t *testing.T, d event.ReviewDecidedEvent) *primitive.MessageExt {
	t.Helper()
	body, err := json.Marshal(d)
	require.NoError(t, err)
	return &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "msg"}
}

func validDecision() event.ReviewDecidedEvent {
	return event.ReviewDecidedEvent{
		BizType: event.ReviewBizAdCreative, ObjectID: 7, Revision: 2, TaskID: 9,
		Purpose: event.ReviewPurposeInitial, Verdict: event.ReviewVerdictApprove,
		PolicyVersion: "v1", Source: event.ReviewSourceHuman, DecidedAt: 1,
	}
}

func TestConsumeSkipsUndecodableAndInvalidDecisions(t *testing.T) {
	invalid := validDecision()
	invalid.Verdict = event.ReviewVerdictReject // 拒绝缺政策码
	applier := &fakeApplier{}

	result := consume(context.Background(), applier,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte("{")}, MsgId: "bad-json"},
		decidedMessage(t, invalid),
		decidedMessage(t, validDecision()),
	)

	require.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, applier.applied, 1)
	require.Equal(t, int64(7), applier.applied[0].ObjectID)
}

func TestConsumeRetriesWhenApplyFails(t *testing.T) {
	applier := &fakeApplier{err: errors.New("db down")}

	result := consume(context.Background(), applier, decidedMessage(t, validDecision()), decidedMessage(t, validDecision()))

	require.Equal(t, consumer.ConsumeRetryLater, result)
	require.Len(t, applier.applied, 1, "the batch stops at the first failure")
}

func TestNewDecidedConsumerSubscribesWithoutConnecting(t *testing.T) {
	_, err := NewDecidedConsumer(&svc.ServiceContext{})
	require.ErrorContains(t, err, "ad-mq: create consumer")

	svcCtx := &svc.ServiceContext{}
	svcCtx.Config.MQ.NameServer, svcCtx.Config.MQ.GroupName = "127.0.0.1:9876", "ad-decision-group"
	c, err := NewDecidedConsumer(svcCtx)
	require.NoError(t, err)
	require.NotNil(t, c)
}
