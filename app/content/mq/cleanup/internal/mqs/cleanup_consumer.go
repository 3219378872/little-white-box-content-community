package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/content/mq/cleanup/internal/store"
	"esx/app/content/mq/cleanup/internal/svc"
	"esx/pkg/event"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

// NewCleanupConsumer 订阅 post-delete，调用 CleanupStore 清理 Redis 相关 key。
// spec §3.2 content-cleanup-consumer，统一处理删除后的脏数据回收。
func NewCleanupConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("cleanup-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeCleanupBatch(ctx, svcCtx.CleanupStore, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicPostDelete, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("cleanup-consumer: subscribe %s: %w", mqx.TopicPostDelete, err)
	}
	return c, nil
}

func consumeCleanupBatch(ctx context.Context, cs store.CleanupStore, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var e event.PostEvent
		if err := json.Unmarshal(msg.Body, &e); err != nil {
			logx.WithContext(ctx).Errorw("cleanup-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if err := e.Validate(); err != nil {
			logx.WithContext(ctx).Errorw("cleanup-consumer: invalid event, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if e.Type != event.PostEventDeleted {
			// Topic 订阅决定了只该收到 deleted 事件；其他类型直接跳过
			continue
		}
		if err := cs.DeletePostState(ctx, e.PostID); err != nil {
			logx.WithContext(ctx).Errorw("cleanup-consumer: delete post state failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		if err := cs.RemoveFromHotZSets(ctx, e.PostID); err != nil {
			logx.WithContext(ctx).Errorw("cleanup-consumer: remove from hot zsets failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		if len(e.Tags) > 0 {
			if err := cs.RemoveFromTagZSets(ctx, e.PostID, e.Tags); err != nil {
				logx.WithContext(ctx).Errorw("cleanup-consumer: remove from tag zsets failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
		}
		logx.WithContext(ctx).Infow("cleanup-consumer: post cleaned",
			logx.Field("post_id", e.PostID), logx.Field("tag_count", len(e.Tags)))
	}
	return consumer.ConsumeSuccess
}
