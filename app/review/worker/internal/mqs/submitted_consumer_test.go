package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/require"

	"esx/app/review/internal/store"
	"esx/app/review/worker/internal/svc"
	"esx/pkg/event"
)

type fakeIngester struct {
	ingested []event.ReviewSubmittedEvent
	err      error
}

func (f *fakeIngester) Ingest(_ context.Context, sub event.ReviewSubmittedEvent) (store.IngestResult, error) {
	f.ingested = append(f.ingested, sub)
	return store.IngestResult{Task: &store.Task{ID: 1}, Created: true}, f.err
}

func submittedMessage(t *testing.T, sub event.ReviewSubmittedEvent) *primitive.MessageExt {
	t.Helper()
	body, err := json.Marshal(sub)
	require.NoError(t, err)
	return &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "msg"}
}

func validSubmission() event.ReviewSubmittedEvent {
	return event.ReviewSubmittedEvent{
		BizType: event.ReviewBizAdCreative, ObjectID: 7, Revision: 1, Purpose: event.ReviewPurposeInitial,
		SubmittedAt: 1, Snapshot: event.ReviewSnapshot{Market: "US", SubmitterID: 3},
	}
}

func TestConsumeSkipsPermanentlyInvalidSubmissions(t *testing.T) {
	missingMarket := validSubmission()
	missingMarket.Snapshot.Market = ""
	ingester := &fakeIngester{}

	result := consume(context.Background(), ingester,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte("not json")}, MsgId: "bad"},
		submittedMessage(t, missingMarket),
		submittedMessage(t, validSubmission()),
	)

	require.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, ingester.ingested, 1)
}

func TestConsumeRetriesTransientIngestFailures(t *testing.T) {
	ingester := &fakeIngester{err: errors.New("db down")}

	result := consume(context.Background(), ingester, submittedMessage(t, validSubmission()), submittedMessage(t, validSubmission()))

	require.Equal(t, consumer.ConsumeRetryLater, result)
	require.Len(t, ingester.ingested, 1)
}

func TestNewSubmittedConsumerSubscribesWithoutConnecting(t *testing.T) {
	_, err := NewSubmittedConsumer(&svc.ServiceContext{})
	require.ErrorContains(t, err, "review-worker: create consumer")

	svcCtx := &svc.ServiceContext{}
	svcCtx.Config.MQ.NameServer, svcCtx.Config.MQ.GroupName = "127.0.0.1:9876", "review-worker-group"
	c, err := NewSubmittedConsumer(svcCtx)
	require.NoError(t, err)
	require.NotNil(t, c)
}
