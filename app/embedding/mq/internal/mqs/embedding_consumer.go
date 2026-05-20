package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/svc"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/event"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

// NewEmbeddingConsumer 订阅 post-create / post-update / post-delete，
// 调用 Embedder + VectorStore 维护 Milvus 向量。spec §3.1 L1 embedding-consumer。
func NewEmbeddingConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("embedding-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeEmbeddingBatch(ctx, svcCtx.Embedder, svcCtx.VectorStore, msgs...), nil
	}
	for _, topic := range []string{mqx.TopicPostCreate, mqx.TopicPostUpdate, mqx.TopicPostDelete} {
		if err := c.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			return nil, fmt.Errorf("embedding-consumer: subscribe %s: %w", topic, err)
		}
	}
	return c, nil
}

func consumeEmbeddingBatch(ctx context.Context, emb embedder.Embedder, vs vectorstore.VectorStore, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var e event.PostEvent
		if err := json.Unmarshal(msg.Body, &e); err != nil {
			logx.WithContext(ctx).Errorw("embedding-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if err := e.Validate(); err != nil {
			logx.WithContext(ctx).Errorw("embedding-consumer: invalid event, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		switch e.Type {
		case event.PostEventCreated, event.PostEventUpdated:
			text := e.Title + "\n" + e.BodyExcerpt
			vec, err := emb.Embed(ctx, text)
			if err != nil {
				logx.WithContext(ctx).Errorw("embedding-consumer: embed failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			if err := vs.Upsert(ctx, e.PostID, vec); err != nil {
				logx.WithContext(ctx).Errorw("embedding-consumer: upsert failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("embedding-consumer: vector upserted",
				logx.Field("post_id", e.PostID), logx.Field("type", string(e.Type)))
		case event.PostEventDeleted:
			if err := vs.Delete(ctx, e.PostID); err != nil {
				logx.WithContext(ctx).Errorw("embedding-consumer: delete failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("post_id", e.PostID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("embedding-consumer: vector deleted",
				logx.Field("post_id", e.PostID))
		}
	}
	return consumer.ConsumeSuccess
}
