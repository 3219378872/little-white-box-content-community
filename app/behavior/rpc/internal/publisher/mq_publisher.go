package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"esx/pkg/event"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/primitive"
)

type Metadata struct {
	TraceID   string
	UserAgent string
}

type Publisher interface {
	Publish(ctx context.Context, behavior event.BehaviorEvent, metadata Metadata) error
}

type Sender interface {
	Send(ctx context.Context, message mqx.Message) (*primitive.SendResult, error)
}

type MQPublisher struct {
	sender Sender
}

func NewMQPublisher(sender Sender) *MQPublisher {
	return &MQPublisher{sender: sender}
}

func (p *MQPublisher) Publish(ctx context.Context, behavior event.BehaviorEvent, metadata Metadata) error {
	body, err := json.Marshal(behavior)
	if err != nil {
		return fmt.Errorf("marshal behavior event: %w", err)
	}
	properties := map[string]string{
		"producer":       behavior.Producer,
		"schema_version": strconv.Itoa(int(behavior.SchemaVersion)),
	}
	if metadata.TraceID != "" {
		properties["trace_id"] = metadata.TraceID
	}
	if metadata.UserAgent != "" {
		properties["user_agent"] = metadata.UserAgent
	}
	_, err = p.sender.Send(ctx, mqx.Message{
		Topic:      mqx.TopicUserBehaviorV2,
		Tag:        mqx.TagDefault,
		Key:        behavior.ClientEventID,
		Body:       body,
		Properties: properties,
	})
	if err != nil {
		return fmt.Errorf("publish behavior event: %w", err)
	}
	return nil
}
