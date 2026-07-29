package mqs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

const canonicalBehaviorJSON = `{"event_id":1,"client_event_id":"client-1","schema_version":2,"event_time":1714300000000,"received_at":1714300000100,"user_id":1,"action":"like","target_id":99,"target_type":"post","producer":"behavior-rpc"}`

type recordingStore struct{ recorded []event.BehaviorEvent }

func (r *recordingStore) Record(_ context.Context, behavior event.BehaviorEvent) error {
	r.recorded = append(r.recorded, behavior)
	return nil
}

type errorStore struct{ err error }

func (e *errorStore) Record(_ context.Context, _ event.BehaviorEvent) error { return e.err }

type recordingDeadLetters struct {
	count int
	err   error
}

func (r *recordingDeadLetters) RecordDeadLetter(context.Context, string, []byte, error) error {
	r.count++
	return r.err
}

type recordingCandidateStore struct {
	posts []event.PostEvent
	err   error
}

func (r *recordingCandidateStore) RecordPost(_ context.Context, post event.PostEvent) error {
	r.posts = append(r.posts, post)
	return r.err
}

func TestRecommendConsumerSkipsMalformedOrInvalidEvents(t *testing.T) {
	for _, body := range []string{`bad`, `{"action":"like"}`} {
		rec := &recordingStore{}
		dead := &recordingDeadLetters{}
		result := consumeBehaviorBatch(context.Background(), rec,
			dead,
			&primitive.MessageExt{Message: primitive.Message{Body: []byte(body)}, MsgId: "bad"})
		assert.Equal(t, consumer.ConsumeSuccess, result)
		assert.Empty(t, rec.recorded)
		assert.Equal(t, 1, dead.count)
	}
}

func TestRecommendConsumerRecordsCanonicalEvent(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&recordingDeadLetters{},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(canonicalBehaviorJSON)}, MsgId: "msg-4"})
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.recorded, 1)
	assert.Equal(t, int64(1), rec.recorded[0].UserID)
	assert.Equal(t, "like", rec.recorded[0].Action)
}

func TestRecommendConsumerStoreErrorReturnsRetry(t *testing.T) {
	result := consumeBehaviorBatch(context.Background(), &errorStore{err: errors.New("store offline")}, &recordingDeadLetters{},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(canonicalBehaviorJSON)}, MsgId: "msg-5"})
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestRecommendConsumerRetriesWhenDeadLetterWriteFails(t *testing.T) {
	result := consumeBehaviorBatch(context.Background(), &recordingStore{},
		&recordingDeadLetters{err: errors.New("redis unavailable")},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "bad"})
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestRecommendConsumerRecordsPostLifecycleCandidates(t *testing.T) {
	post := event.PostEvent{
		EventID: 9, EventTime: 10, Type: event.PostEventCreated,
		PostID: 99, AuthorID: 7, Title: "post",
	}
	body, _ := json.Marshal(post)
	candidates := &recordingCandidateStore{}
	result := consumePostBatch(context.Background(), candidates, &recordingDeadLetters{},
		&primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "post-1"})
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Equal(t, []event.PostEvent{post}, candidates.posts)
}

func TestRecommendConsumerDeadLettersMalformedOrInvalidPostEvents(t *testing.T) {
	for _, body := range [][]byte{[]byte(`bad`), []byte(`{"post_id":99}`)} {
		dead := &recordingDeadLetters{}
		result := consumePostBatch(context.Background(), &recordingCandidateStore{}, dead,
			&primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "bad-post"})

		assert.Equal(t, consumer.ConsumeSuccess, result)
		assert.Equal(t, 1, dead.count)
	}
}

func TestRecommendConsumerPostDeadLetterFailureReturnsRetry(t *testing.T) {
	result := consumePostBatch(context.Background(), &recordingCandidateStore{},
		&recordingDeadLetters{err: errors.New("dead letter unavailable")},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "bad-post"})

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestRecommendConsumerCandidateStoreFailureReturnsRetry(t *testing.T) {
	post := event.PostEvent{
		EventID: 9, EventTime: 10, Type: event.PostEventCreated,
		PostID: 99, AuthorID: 7, Title: "post",
	}
	body, err := json.Marshal(post)
	assert.NoError(t, err)
	result := consumePostBatch(context.Background(),
		&recordingCandidateStore{err: errors.New("candidate store unavailable")},
		&recordingDeadLetters{},
		&primitive.MessageExt{Message: primitive.Message{Body: body}, MsgId: "post-1"})

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}
