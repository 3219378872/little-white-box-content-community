package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"esx/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisCandidateStoreWritesVersionedPostKeys(t *testing.T) {
	redis := &fakeEvaler{}
	post := event.PostEvent{
		EventID: 1, EventTime: 100, Type: event.PostEventCreated,
		PostID: 9, AuthorID: 4, Tags: []string{"go"},
	}

	err := NewRedisCandidateStore(redis, "v2", "recommend", 3600).
		RecordPost(context.Background(), post)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"feature:v2:post:9",
		"recommend:v2:recall:post:hot:home",
		"recommend:v2:recall:post:explore:home",
		"recommend:v2:author:4:posts",
		"recommend:v2:follow:author:4:followers",
	}, redis.keys)
	assert.Equal(t, "post.created", redis.args[0])
	assert.Equal(t, "go", redis.args[3])
}

func TestRedisCandidateStorePropagatesFailure(t *testing.T) {
	redis := &fakeEvaler{err: errors.New("redis down")}
	post := event.PostEvent{EventID: 1, Type: event.PostEventDeleted, PostID: 9}
	err := NewRedisCandidateStore(redis, "v2", "recommend", 3600).
		RecordPost(context.Background(), post)
	assert.ErrorContains(t, err, "redis down")
}

func TestRedisDeadLetterRecorderUsesBoundedVersionedList(t *testing.T) {
	redis := &fakeEvaler{}
	recorder := NewRedisDeadLetterRecorder(redis, "recommend", "v2", 600, 25)
	recorder.now = func() time.Time { return time.UnixMilli(123) }

	require.NoError(t, recorder.RecordDeadLetter(
		context.Background(), "message-1", []byte(`bad`), errors.New("invalid json"),
	))

	assert.Equal(t, []string{"recommend:v2:dead-letters"}, redis.keys)
	assert.Contains(t, redis.args[0], `"message_id":"message-1"`)
	assert.Equal(t, 25, redis.args[1])
	assert.Equal(t, 600, redis.args[2])
}
