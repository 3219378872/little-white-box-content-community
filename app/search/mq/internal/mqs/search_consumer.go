package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/search/mq/internal/indexer"
	"esx/app/search/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type searchEvent struct {
	Type  string         `json:"type"`
	DocID string         `json:"doc_id"`
	Body  map[string]any `json:"body"`
}

func NewSearchConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("search-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeSearchBatch(ctx, svcCtx.Indexer, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicSearchIndex, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("search-consumer: subscribe %s: %w", mqx.TopicSearchIndex, err)
	}
	return c, nil
}

func consumeSearchBatch(ctx context.Context, idx indexer.Indexer, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var event searchEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("search-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.DocID == "" {
			logx.WithContext(ctx).Errorw("search-consumer: missing doc_id",
				logx.Field("msg_id", msg.MsgId), logx.Field("type", event.Type))
			continue
		}
		switch event.Type {
		case "index":
			if err := idx.Index(ctx, indexer.IndexDoc{
				DocID: event.DocID, Type: event.Type, Body: event.Body,
			}); err != nil {
				logx.WithContext(ctx).Errorw("search-consumer: index failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("doc_id", event.DocID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("search-consumer: document indexed",
				logx.Field("doc_id", event.DocID))
		case "delete":
			if err := idx.Delete(ctx, event.DocID); err != nil {
				logx.WithContext(ctx).Errorw("search-consumer: delete failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("doc_id", event.DocID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("search-consumer: document deleted",
				logx.Field("doc_id", event.DocID))
		default:
			logx.WithContext(ctx).Errorw("search-consumer: unknown event type, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("type", event.Type))
		}
	}
	return consumer.ConsumeSuccess
}
