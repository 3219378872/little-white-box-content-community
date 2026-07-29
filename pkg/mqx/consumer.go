package mqx

import (
	"context"
	"fmt"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// ConsumerConfig 消费者配置
type ConsumerConfig struct {
	NameServer        string // NameServer 地址
	GroupName         string // 消费者组名
	Topic             string // 订阅 Topic
	Tag               string // 订阅 Tag
	ConsumeOrder      bool   // 是否顺序消费
	MaxReconsumeTimes int32  // 最大重试次数
}

// Consumer RocketMQ 消费者封装
type Consumer struct {
	c      rocketmq.PushConsumer
	config ConsumerConfig
}

// MessageHandler 消息处理函数
type MessageHandler func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error)

// NewConsumer 创建消费者
func NewConsumer(config ConsumerConfig) (*Consumer, error) {
	opts := []consumer.Option{
		consumer.WithNameServer([]string{config.NameServer}),
		consumer.WithGroupName(config.GroupName),
	}

	if config.ConsumeOrder {
		opts = append(opts, consumer.WithConsumerOrder(true))
	}
	if config.MaxReconsumeTimes > 0 {
		opts = append(opts, consumer.WithMaxReconsumeTimes(config.MaxReconsumeTimes))
	}

	c, err := rocketmq.NewPushConsumer(opts...)
	if err != nil {
		return nil, err
	}

	return &Consumer{c: c, config: config}, nil
}

// Subscribe 订阅消息
func (c *Consumer) Subscribe(handler MessageHandler) error {
	if c.config.Topic == "" {
		return fmt.Errorf("mqx: consumer topic is required")
	}
	return c.SubscribeWithTopic(c.config.Topic, c.config.Tag, handler)
}

// SubscribeWithTopic 订阅指定 Topic
func (c *Consumer) SubscribeWithTopic(topic, tag string, handler MessageHandler) error {
	selector := consumer.MessageSelector{}
	if tag != "" {
		selector = consumer.MessageSelector{
			Type:       consumer.TAG,
			Expression: tag,
		}
	}
	return c.c.Subscribe(topic, selector, func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return handler(ctx, msgs...)
	})
}

// Start 启动消费者
func (c *Consumer) Start() error {
	return c.c.Start()
}

// Shutdown 关闭消费者
func (c *Consumer) Shutdown() error {
	return c.c.Shutdown()
}
