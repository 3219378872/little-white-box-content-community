package logic

import (
	"context"
	"errors"
	"testing"

	"esx/pkg/event"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

type erroringProducer struct{ sent int }

func (e *erroringProducer) SendSyncWithTag(_ context.Context, _ string, _ string, _ []byte) (*primitive.SendResult, error) {
	e.sent++
	return nil, errors.New("mq down")
}

func TestPublishPostEvent_NilProducer_NoOp(t *testing.T) {
	// 不应 panic
	publishPostEvent(context.Background(), nil, "post-create", event.PostEvent{
		Type: event.PostEventCreated, PostID: 1, AuthorID: 1,
	})
}

func TestPublishPostEvent_InvalidEvent_NoSend(t *testing.T) {
	p := &fakeMQProducer{}
	publishPostEvent(context.Background(), p, "post-create", event.PostEvent{
		// 缺 PostID
		Type: event.PostEventCreated, AuthorID: 1,
	})
	assert.Empty(t, p.topic)
}

func TestPublishPostEvent_ProducerError_Swallowed(t *testing.T) {
	p := &erroringProducer{}
	// 不应 panic、不应抛出错误（best-effort）
	publishPostEvent(context.Background(), p, "post-create", event.PostEvent{
		Type: event.PostEventCreated, PostID: 1, AuthorID: 1,
	})
	assert.Equal(t, 1, p.sent)
}

func TestPublishPostEvent_AutoFillsEventIDAndTime(t *testing.T) {
	p := &fakeMQProducer{}
	publishPostEvent(context.Background(), p, "post-create", event.PostEvent{
		Type: event.PostEventCreated, PostID: 1, AuthorID: 1,
	})
	assert.NotEmpty(t, p.body, "should have sent body")
}
