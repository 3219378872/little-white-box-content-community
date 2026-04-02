package mqx

import (
	"context"

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
	p, err := rocketmq.NewProducer(
		producer.WithNameServer([]string{config.NameServer}),
		producer.WithGroupName(config.GroupName),
		producer.WithRetry(config.Retry),
	)
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
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	return p.p.SendSync(ctx, msg)
}

// SendSyncWithKey 同步发送消息带 Key
func (p *Producer) SendSyncWithKey(ctx context.Context, topic, key string, body []byte) (*primitive.SendResult, error) {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	msg.WithKeys([]string{key})
	return p.p.SendSync(ctx, msg)
}

// SendSyncWithTag 同步发送消息带 Tag
func (p *Producer) SendSyncWithTag(ctx context.Context, topic, tag string, body []byte) (*primitive.SendResult, error) {
	msg := &primitive.Message{
		Topic: topic,
		Body:  body,
	}
	msg.WithTag(tag)
	return p.p.SendSync(ctx, msg)
}

// SendAsync 异步发送消息
func (p *Producer) SendAsync(ctx context.Context, topic string, body []byte, callback func(result *primitive.SendResult, err error)) error {
	panic("未实现")
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
