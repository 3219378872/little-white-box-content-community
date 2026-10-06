package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// Metadata 是随消息属性携带、不进入事件正文的请求上下文。
type Metadata struct {
	TraceID   string
	UserAgent string
}

// Publisher 投递一条行为事件。
type Publisher interface {
	Publish(ctx context.Context, behavior event.BehaviorEvent, metadata Metadata) error
}

// Sender 是 MQ 生产者的发送接口，便于测试替换。
type Sender interface {
	Send(ctx context.Context, message mqx.Message) (*primitive.SendResult, error)
}

// MQPublisher 把行为事件发送到 user-behavior-v2 主题。
type MQPublisher struct {
	sender Sender
}

// NewMQPublisher 用给定发送器创建投递器。
func NewMQPublisher(sender Sender) *MQPublisher {
	return &MQPublisher{sender: sender}
}

// Publish 以 client_event_id 作为消息 Key 发送事件，便于按客户端事件追查；
// 生产者与 schema 版本写入消息属性，消费端无需解析正文即可路由。
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
