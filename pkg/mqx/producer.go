package mqx

import (
	"context"
	"fmt"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
)

// ProducerConfig 生产者配置
type ProducerConfig struct {
	NameServer  string // NameServer 地址
	GroupName   string // 生产者组名
	Retry       int    // 重试次数
	SendTimeout int    // 发送超时(毫秒)
}

// Producer RocketMQ 生产者封装
type Producer struct {
	p rocketmq.Producer
}

// NewProducer 创建生产者
func NewProducer(config ProducerConfig) (*Producer, error) {
	opts := []producer.Option{
		producer.WithNameServer([]string{config.NameServer}),
		producer.WithGroupName(config.GroupName),
		producer.WithRetry(config.Retry),
	}
	if config.SendTimeout > 0 {
		opts = append(opts, producer.WithSendMsgTimeout(time.Duration(config.SendTimeout)*time.Millisecond))
	}
	p, err := rocketmq.NewProducer(opts...)
	if err != nil {
		return nil, err
	}

	if err := p.Start(); err != nil {
		return nil, err
	}

	return &Producer{p: p}, nil
}

// SendSync 同步发送消息
func (p *Producer) SendSync(ctx context.Context, topic string, body []byte) (*primitive.SendResult, error) {
	return p.Send(ctx, Message{Topic: topic, Body: body})
}

// SendSyncWithKey 同步发送消息带 Key
func (p *Producer) SendSyncWithKey(ctx context.Context, topic, key string, body []byte) (*primitive.SendResult, error) {
	return p.Send(ctx, Message{Topic: topic, Key: key, Body: body})
}

// SendSyncWithTag 同步发送消息带 Tag
func (p *Producer) SendSyncWithTag(ctx context.Context, topic, tag string, body []byte) (*primitive.SendResult, error) {
	return p.Send(ctx, Message{Topic: topic, Tag: tag, Body: body})
}

// Message contains the transport metadata needed by event producers.
type Message struct {
	Topic      string
	Tag        string
	Key        string
	Body       []byte
	Properties map[string]string
}

// BuildMessage converts the transport-neutral input into a RocketMQ message.
func BuildMessage(input Message) (*primitive.Message, error) {
	if input.Topic == "" {
		return nil, fmt.Errorf("mqx: topic is required")
	}
	if len(input.Body) == 0 {
		return nil, fmt.Errorf("mqx: body is required")
	}
	msg := &primitive.Message{Topic: input.Topic, Body: input.Body}
	if len(input.Properties) > 0 {
		msg.WithProperties(input.Properties)
	}
	if input.Tag != "" {
		msg.WithTag(input.Tag)
	}
	if input.Key != "" {
		msg.WithKeys([]string{input.Key})
	}
	return msg, nil
}

// Send synchronously publishes a message and treats non-SendOK broker replies
// as failures. Callers may only acknowledge work after this method succeeds.
func (p *Producer) Send(ctx context.Context, input Message) (*primitive.SendResult, error) {
	msg, err := BuildMessage(input)
	if err != nil {
		return nil, err
	}
	result, err := p.p.SendSync(ctx, msg)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Status != primitive.SendOK {
		return result, fmt.Errorf("mqx: broker did not acknowledge message")
	}
	return result, nil
}

// SendAsync 异步发送消息
func (p *Producer) SendAsync(ctx context.Context, topic string, body []byte, callback func(result *primitive.SendResult, err error)) error {
	msg, err := BuildMessage(Message{Topic: topic, Body: body})
	if err != nil {
		return err
	}
	return p.p.SendAsync(ctx, func(_ context.Context, result *primitive.SendResult, sendErr error) {
		if sendErr == nil && (result == nil || result.Status != primitive.SendOK) {
			sendErr = fmt.Errorf("mqx: broker did not acknowledge message")
		}
		callback(result, sendErr)
	}, msg)
}

// SendOneWay 单向发送消息(不关心结果)
func (p *Producer) SendOneWay(ctx context.Context, topic string, body []byte) error {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	return p.p.SendOneWay(ctx, msg)
}

// Shutdown 关闭生产者
func (p *Producer) Shutdown() error {
	return p.p.Shutdown()
}
