package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/feed/rpc/internal/fanout"
	"esx/app/feed/rpc/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type postPublishedMessage struct {
	PostId    int64 `json:"post_id"`
	AuthorId  int64 `json:"author_id"`
	CreatedAt int64 `json:"created_at"`
}

func NewPostPublishConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("create post publish consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		for _, msg := range msgs {
			var event postPublishedMessage
			if err := json.Unmarshal(msg.Body, &event); err != nil {
				logx.WithContext(ctx).Errorw("unmarshal post created event failed", logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
				continue
			}
			if err := handlePostPublished(ctx, svcCtx, event); err != nil {
				logx.WithContext(ctx).Errorw("handle post created event failed", logx.Field("post_id", event.PostId), logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater, nil
			}
		}
		return consumer.ConsumeSuccess, nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicPostCreate, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("subscribe post create topic: %w", err)
	}
	return c, nil
}

func handlePostPublished(ctx context.Context, svcCtx *svc.ServiceContext, event postPublishedMessage) error {
	_, err := fanout.HandlePostPublished(ctx, svcCtx, fanout.PostPublished{
		PostId:    event.PostId,
		AuthorId:  event.AuthorId,
		CreatedAt: event.CreatedAt,
	})
	return err
}
