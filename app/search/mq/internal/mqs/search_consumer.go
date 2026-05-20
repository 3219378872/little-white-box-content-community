package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"esx/app/search/mq/internal/indexer"
	"esx/app/search/mq/internal/svc"
	"esx/pkg/event"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

// NewSearchConsumer 订阅 post-create / post-update / post-delete 三个 topic，
// 由统一 handler 按事件类型分发到 indexer。spec §3.1 L1 search-index-consumer。
func NewSearchConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("search-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeSearchBatch(ctx, svcCtx.Indexer, msgs...), nil
	}
	for _, topic := range []string{mqx.TopicPostCreate, mqx.TopicPostUpdate, mqx.TopicPostDelete} {
		if err := c.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			return nil, fmt.Errorf("search-consumer: subscribe %s: %w", topic, err)
		}
	}
	return c, nil
}

func consumeSearchBatch(ctx context.Context, idx indexer.Indexer, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var e event.PostEvent
		if err := json.Unmarshal(msg.Body, &e); err != nil {
			logx.WithContext(ctx).Errorw("search-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if err := e.Validate(); err != nil {
			logx.WithContext(ctx).Errorw("search-consumer: invalid event, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		switch e.Type {
		case event.PostEventCreated, event.PostEventUpdated:
			doc := indexer.PostEventToIndexDoc(e)
			if err := idx.Index(ctx, doc); err != nil {
				logx.WithContext(ctx).Errorw("search-consumer: index failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("search-consumer: document indexed",
				logx.Field("post_id", e.PostID), logx.Field("type", string(e.Type)))
		case event.PostEventDeleted:
			docID := strconv.FormatInt(e.PostID, 10)
			if err := idx.Delete(ctx, docID); err != nil {
				logx.WithContext(ctx).Errorw("search-consumer: delete failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("search-consumer: document deleted",
				logx.Field("post_id", e.PostID))
		}
	}
	return consumer.ConsumeSuccess
}
