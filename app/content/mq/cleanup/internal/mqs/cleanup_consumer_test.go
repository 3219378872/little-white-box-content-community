package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubCleanupStore struct {
	mu           sync.Mutex
	deletedPosts []int64
	hotRemovals  []int64
	tagRemovals  map[int64][]string
	stateErr     error
	hotErr       error
	tagErr       error
}

func newStub() *stubCleanupStore {
	return &stubCleanupStore{tagRemovals: map[int64][]string{}}
}

func (s *stubCleanupStore) DeletePostState(_ context.Context, postID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stateErr != nil {
		return s.stateErr
	}
	s.deletedPosts = append(s.deletedPosts, postID)
	return nil
}
func (s *stubCleanupStore) RemoveFromHotZSets(_ context.Context, postID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hotErr != nil {
		return s.hotErr
	}
	s.hotRemovals = append(s.hotRemovals, postID)
	return nil
}
func (s *stubCleanupStore) RemoveFromTagZSets(_ context.Context, postID int64, tags []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tagErr != nil {
		return s.tagErr
	}
	s.tagRemovals[postID] = append(s.tagRemovals[postID], tags...)
	return nil
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func mq(id string, body []byte) *primitive.MessageExt {
	return &primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: id}
}

func TestCleanupConsumer_PostDeleted_RunsAllCleanup(t *testing.T) {
	stub := newStub()
	e := event.PostEvent{
		EventID: 1, EventTime: 1, Type: event.PostEventDeleted,
		PostID: 999, Tags: []string{"a", "b"},
	}
	res := consumeCleanupBatch(context.Background(), stub, mq("m1", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Equal(t, []int64{999}, stub.deletedPosts)
	assert.Equal(t, []int64{999}, stub.hotRemovals)
	assert.Equal(t, []string{"a", "b"}, stub.tagRemovals[999])
}

func TestCleanupConsumer_NoTags_SkipsTagCleanup(t *testing.T) {
	stub := newStub()
	e := event.PostEvent{
		EventID: 2, EventTime: 2, Type: event.PostEventDeleted, PostID: 1000,
	}
	res := consumeCleanupBatch(context.Background(), stub, mq("m2", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Equal(t, []int64{1000}, stub.deletedPosts)
	assert.Empty(t, stub.tagRemovals)
}

func TestCleanupConsumer_NonDeleteEvent_Skips(t *testing.T) {
	stub := newStub()
	e := event.PostEvent{
		EventID: 3, EventTime: 3, Type: event.PostEventCreated, PostID: 1001, AuthorID: 1,
	}
	res := consumeCleanupBatch(context.Background(), stub, mq("m3", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Empty(t, stub.deletedPosts)
}

func TestCleanupConsumer_InvalidJSON_Skips(t *testing.T) {
	stub := newStub()
	res := consumeCleanupBatch(context.Background(), stub, mq("m4", []byte(`bad`)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Empty(t, stub.deletedPosts)
}

func TestCleanupConsumer_DeletePostStateError_Retries(t *testing.T) {
	stub := newStub()
	stub.stateErr = errors.New("redis down")
	e := event.PostEvent{EventID: 5, EventTime: 5, Type: event.PostEventDeleted, PostID: 1002}
	res := consumeCleanupBatch(context.Background(), stub, mq("m5", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestCleanupConsumer_HotZSetError_Retries(t *testing.T) {
	stub := newStub()
	stub.hotErr = errors.New("redis timeout")
	e := event.PostEvent{EventID: 6, EventTime: 6, Type: event.PostEventDeleted, PostID: 1003}
	res := consumeCleanupBatch(context.Background(), stub, mq("m6", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestCleanupConsumer_TagZSetError_Retries(t *testing.T) {
	stub := newStub()
	stub.tagErr = errors.New("zrem error")
	e := event.PostEvent{
		EventID: 7, EventTime: 7, Type: event.PostEventDeleted, PostID: 1004,
		Tags: []string{"x"},
	}
	res := consumeCleanupBatch(context.Background(), stub, mq("m7", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeRetryLater, res)
}

func TestCleanupConsumer_InvalidEvent_Skips(t *testing.T) {
	stub := newStub()
	e := event.PostEvent{EventID: 0, Type: event.PostEventDeleted, PostID: 1}
	res := consumeCleanupBatch(context.Background(), stub, mq("m8", mustMarshal(t, e)))
	assert.Equal(t, consumer.ConsumeSuccess, res)
	assert.Empty(t, stub.deletedPosts)
}
