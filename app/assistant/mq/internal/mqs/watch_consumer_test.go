package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"esx/app/assistant/watch"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errStore struct {
	watch.MapStore
	listErr error
}

func (s *errStore) ListEnabled(ctx context.Context) ([]watch.Task, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.MapStore.ListEnabled(ctx)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func msg(id string, body []byte) *primitive.MessageExt {
	return &primitive.MessageExt{Body: body, MsgId: id}
}

func TestConsumeWatchBatch_MalformedJSON_Skips(t *testing.T) {
	store := watch.NewMapStore()
	result := consumeWatchBatch(context.Background(), matcher{Store: store}, msg("m1", []byte(`bad`)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestConsumeWatchBatch_PublishedCreate_RecordsHit(t *testing.T) {
	store := watch.NewMapStore()
	_, err := store.Create(context.Background(), watch.Task{
		UserID: 3, ConditionType: watch.AuthorNewPost, TargetType: "author", TargetID: 4,
	})
	require.NoError(t, err)
	ev := event.PostEvent{
		EventID: 9, EventTime: 100, Type: event.PostEventCreated,
		PostID: 21, AuthorID: 4, Title: "新帖", Status: 1,
	}
	result := consumeWatchBatch(context.Background(), matcher{Store: store}, msg("m2", mustMarshal(t, ev)))
	assert.Equal(t, consumer.ConsumeSuccess, result)
	hits, err := store.ListHits(context.Background(), 3, true)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, int64(21), hits[0].PostID)
}

func TestConsumeWatchBatch_StoreError_Retries(t *testing.T) {
	store := &errStore{listErr: errors.New("db down")}
	ev := event.PostEvent{
		EventID: 1, EventTime: 1, Type: event.PostEventCreated,
		PostID: 2, AuthorID: 3, Status: 1,
	}
	result := consumeWatchBatch(context.Background(), matcher{Store: store}, msg("m3", mustMarshal(t, ev)))
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestConsumeWatchBatch_CommentBelowSpikeThreshold_NoHit(t *testing.T) {
	store := watch.NewMapStore()
	_, err := store.Create(context.Background(), watch.Task{
		UserID: 3, ConditionType: watch.DiscussionSpike, TargetType: "post", TargetID: 21,
	})
	require.NoError(t, err)
	judgeCalls := 0
	m := matcher{
		Store:            store,
		SpikeMinComments: 5,
		SpikeJudge: func(context.Context, watch.Task, int64, int) (bool, error) {
			judgeCalls++
			return true, nil
		},
	}
	ev := event.BehaviorEvent{
		EventID: 4, ClientEventID: "c1", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, Producer: "test", UserID: 9,
		Action: event.BehaviorActionComment, TargetID: 21, TargetType: "post",
	}
	body := mustMarshal(t, ev)
	msg := msg("m4", body)
	msg.Topic = mqx.TopicUserBehaviorV2
	result := consumeWatchBatch(context.Background(), m, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Equal(t, 0, judgeCalls)
	hits, err := store.ListHits(context.Background(), 3, false)
	require.NoError(t, err)
	assert.Len(t, hits, 0)
}

func TestWatchEventLagSeconds(t *testing.T) {
	now := time.UnixMilli(2000)
	assert.Equal(t, 0.0, watchEventLagSeconds(0, now))
	assert.Equal(t, 1.0, watchEventLagSeconds(1000, now))
	assert.Equal(t, 0.0, watchEventLagSeconds(3000, now))
}
