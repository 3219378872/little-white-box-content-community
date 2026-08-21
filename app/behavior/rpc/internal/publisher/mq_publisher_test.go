package publisher

import (
	"context"
	"encoding/json"
	"testing"

	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	message mqx.Message
	err     error
}

func (s *fakeSender) Send(_ context.Context, message mqx.Message) (*primitive.SendResult, error) {
	s.message = message
	if s.err != nil {
		return nil, s.err
	}
	return &primitive.SendResult{Status: primitive.SendOK}, nil
}

func TestMQPublisherPublishesCanonicalEventWithMetadata(t *testing.T) {
	sender := &fakeSender{}
	p := NewMQPublisher(sender)
	behavior := event.BehaviorEvent{
		EventID: 1, ClientEventID: "client-1", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 101, UserID: 9, Action: event.BehaviorActionLike,
		TargetID: 7, TargetType: "post", Producer: "behavior-rpc",
	}

	require.NoError(t, p.Publish(context.Background(), behavior, Metadata{
		TraceID: "trace-1", UserAgent: "test-agent",
	}))
	assert.Equal(t, mqx.TopicUserBehaviorV2, sender.message.Topic)
	assert.Equal(t, mqx.TagDefault, sender.message.Tag)
	assert.Equal(t, "client-1", sender.message.Key)
	assert.Equal(t, "trace-1", sender.message.Properties["trace_id"])
	assert.Equal(t, "test-agent", sender.message.Properties["user_agent"])

	var got event.BehaviorEvent
	require.NoError(t, json.Unmarshal(sender.message.Body, &got))
	assert.Equal(t, behavior, got)
}
