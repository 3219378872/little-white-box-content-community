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
	"esx/pkg/mqx"
	"esx/pkg/visibilityx"

	"esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
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
			logging.WithContext(ctx).Errorw("search-consumer: unmarshal failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			searchConsumerMessages.Inc("invalid")
			continue
		}
		if err := e.Validate(); err != nil {
			logging.WithContext(ctx).Errorw("search-consumer: invalid event, skipping",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			searchConsumerMessages.Inc("invalid")
			continue
		}
		if e.Type == event.PostEventCounted {
			if err := idx.PatchCounts(ctx, indexer.PostEventToIndexDoc(e)); err != nil {
				logging.WithContext(ctx).Errorw("search-consumer: count patch failed",
					logging.Field("msg_id", msg.MsgId), logging.Field("post_id", e.PostID),
					logging.Field("err", err.Error()))
				searchConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
			searchConsumerMessages.Inc("processed")
			observeSearchIndexLag(e.EventTime, time.Now())
			continue
		}
		switch e.Type {
		case event.PostEventCreated, event.PostEventUpdated:
			// CORE-015：草稿/取消发布的内容不得进入搜索索引。
			if !visibilityx.IsPublished(int32(e.Status)) {
				docID := strconv.FormatInt(e.PostID, 10)
				if err := idx.Delete(ctx, docID, e.Revision); err != nil {
					logging.WithContext(ctx).Errorw("search-consumer: delete non-published doc failed",
						logging.Field("msg_id", msg.MsgId), logging.Field("post_id", e.PostID),
						logging.Field("err", err.Error()))
					searchConsumerMessages.Inc("retry")
					return consumer.ConsumeRetryLater
				}
				logging.WithContext(ctx).Infow("search-consumer: non-published doc removed",
					logging.Field("post_id", e.PostID), logging.Field("status", e.Status))
				searchConsumerMessages.Inc("processed")
				observeSearchIndexLag(e.EventTime, time.Now())
				continue
			}
			doc := indexer.PostEventToIndexDoc(e)
			if err := idx.Index(ctx, doc); err != nil {
				logging.WithContext(ctx).Errorw("search-consumer: index failed",
					logging.Field("msg_id", msg.MsgId), logging.Field("post_id", e.PostID),
					logging.Field("err", err.Error()))
				searchConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
			logging.WithContext(ctx).Infow("search-consumer: document indexed",
				logging.Field("post_id", e.PostID), logging.Field("type", string(e.Type)))
		case event.PostEventDeleted:
			docID := strconv.FormatInt(e.PostID, 10)
			if err := idx.Delete(ctx, docID, e.Revision); err != nil {
				logging.WithContext(ctx).Errorw("search-consumer: delete failed",
					logging.Field("msg_id", msg.MsgId), logging.Field("post_id", e.PostID),
					logging.Field("err", err.Error()))
				searchConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
			logging.WithContext(ctx).Infow("search-consumer: document deleted",
				logging.Field("post_id", e.PostID))
		}
		searchConsumerMessages.Inc("processed")
		observeSearchIndexLag(e.EventTime, time.Now())
	}
	return consumer.ConsumeSuccess
}
