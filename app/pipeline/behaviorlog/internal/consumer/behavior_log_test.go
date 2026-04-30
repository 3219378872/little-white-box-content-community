package consumer

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/event"
	"util"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

func init() {
	_ = util.InitSnowflake(1, 1)
}

type mockStore struct {
	inserted []event.BehaviorEvent
	err      error
}

func (m *mockStore) Insert(_ context.Context, e event.BehaviorEvent) error {
	if m.err != nil {
		return m.err
	}
	m.inserted = append(m.inserted, e)
	return nil
}

type mockDedup struct {
	seen map[string]bool
	err  error
}

func newMockDedup() *mockDedup {
	return &mockDedup{seen: make(map[string]bool)}
}

func (m *mockDedup) IsDuplicate(_ context.Context, eventID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.seen[eventID] {
		return true, nil
	}
	m.seen[eventID] = true
	return false, nil
}

func makeMsg(body string) *primitive.MessageExt {
	return &primitive.MessageExt{
		Message: primitive.Message{Body: []byte(body)},
		MsgId:   "test-msg",
	}
}

func TestConsumeBehavior_ValidEvent_Inserts(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.inserted, 1)
	assert.Equal(t, int64(42), store.inserted[0].UserID)
	assert.Equal(t, "like", store.inserted[0].Action)
}

func TestConsumeBehavior_MalformedJSON_Skips(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`bad-json`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.inserted)
}

func TestConsumeBehavior_ValidationFails_Skips(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"user_id":0,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.inserted)
}

func TestConsumeBehavior_DuplicateEvent_Skips(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	msg := makeMsg(`{"event_id":100,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`)

	result1 := consumeBehaviorMsg(context.Background(), store, dedup, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result1)
	assert.Len(t, store.inserted, 1)

	result2 := consumeBehaviorMsg(context.Background(), store, dedup, msg)
	assert.Equal(t, consumer.ConsumeSuccess, result2)
	assert.Len(t, store.inserted, 1)
}

func TestConsumeBehavior_StoreError_ReturnsRetry(t *testing.T) {
	store := &mockStore{err: errors.New("clickhouse down")}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestConsumeBehavior_DedupError_FallsThrough(t *testing.T) {
	store := &mockStore{}
	dedup := &mockDedup{seen: make(map[string]bool), err: errors.New("redis down")}

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"event_id":1,"event_time":1714300000000,"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.inserted, 1)
}

func TestConsumeBehavior_ZeroEventID_GeneratesOne(t *testing.T) {
	store := &mockStore{}
	dedup := newMockDedup()

	result := consumeBehaviorMsg(context.Background(), store, dedup,
		makeMsg(`{"user_id":42,"action":"like","target_id":999,"target_type":"post"}`),
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.inserted, 1)
	assert.NotZero(t, store.inserted[0].EventID)
}
