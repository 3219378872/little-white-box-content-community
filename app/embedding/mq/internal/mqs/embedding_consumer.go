package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/content/rpc/contentservice"
	"esx/app/content/visibility"
	"esx/app/embedding/mq/internal/embedder"
	"esx/app/embedding/mq/internal/svc"
	"esx/app/embedding/mq/internal/vectorstore"
	"esx/pkg/event"
	"esx/pkg/mqx"
	"esx/pkg/visibilityx"

	"esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// NewEmbeddingConsumer 订阅 post-create / post-update / post-delete，
// 调用 Embedder + VectorStore 维护 Milvus 向量。spec §3.1 L1 embedding-consumer。
func NewEmbeddingConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("embedding-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeEmbeddingBatch(ctx, svcCtx.Embedder, svcCtx.VectorStore, svcCtx.Content, msgs...), nil
	}
	for _, topic := range []string{mqx.TopicPostCreate, mqx.TopicPostUpdate, mqx.TopicPostDelete} {
		if err := c.SubscribeWithTopic(topic, mqx.TagDefault, handler); err != nil {
			return nil, fmt.Errorf("embedding-consumer: subscribe %s: %w", topic, err)
		}
	}
	return c, nil
}

func consumeEmbeddingBatch(ctx context.Context, emb embedder.Embedder, vs vectorstore.VectorStore, source visibility.PostsByIDs, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var e event.PostEvent
		if err := json.Unmarshal(msg.Body, &e); err != nil {
			logging.WithContext(ctx).Errorw("embedding-consumer: unmarshal failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			embeddingConsumerMessages.Inc("invalid")
			continue
		}
		if err := e.Validate(); err != nil {
			logging.WithContext(ctx).Errorw("embedding-consumer: invalid event, skipping",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			embeddingConsumerMessages.Inc("invalid")
			continue
		}
		if e.Type == event.PostEventCounted {
			embeddingConsumerMessages.Inc("processed")
			continue
		}
		storedRevision, err := vs.CurrentRevision(ctx, e.PostID)
		if err != nil {
			logging.WithContext(ctx).Errorw("embedding-consumer: read revision failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("post_id", e.PostID),
				logging.Field("err", err.Error()))
			embeddingConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		if !event.ReplacesStoredRevision(e.Revision, storedRevision) {
			logging.WithContext(ctx).Infow("embedding-consumer: stale revision skipped",
				logging.Field("post_id", e.PostID), logging.Field("revision", e.Revision),
				logging.Field("stored_revision", storedRevision))
			embeddingConsumerMessages.Inc("processed")
			continue
		}
		// Reconcile from Content authority even on historical replay. Rebuilds
		// contain published rows only, so an absent old tombstone cannot justify
		// trusting an old event body after alias promotion.
		post, err := authoritativePost(ctx, source, e.PostID)
		if err != nil {
			logging.WithContext(ctx).Errorw("embedding-consumer: authority lookup failed", logging.Field("post_id", e.PostID), logging.Field("err", err.Error()))
			embeddingConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		if post == nil {
			if err := vs.Delete(ctx, e.PostID, max(e.Revision, storedRevision, 1)); err != nil {
				logging.WithContext(ctx).Errorw("embedding-consumer: tombstone failed", logging.Field("post_id", e.PostID), logging.Field("err", err.Error()))
				embeddingConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
		} else {
			if post.Revision < max(e.Revision, 1) {
				// Do not acknowledge an event from a newer authority snapshot
				// against a lagging Content endpoint.
				embeddingConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
			result, err := emb.Embed(ctx, post.Title+"\n"+post.Content)
			if err != nil {
				logging.WithContext(ctx).Errorw("embedding-consumer: embed failed", logging.Field("post_id", e.PostID), logging.Field("err", err.Error()))
				embeddingConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
			if err := vs.Upsert(ctx, vectorstore.Record{PostID: e.PostID, Vector: result.Vector, ModelVersion: result.ModelVersion, Dimension: result.Dimension, Revision: post.Revision}); err != nil {
				logging.WithContext(ctx).Errorw("embedding-consumer: upsert failed", logging.Field("post_id", e.PostID), logging.Field("err", err.Error()))
				embeddingConsumerMessages.Inc("retry")
				return consumer.ConsumeRetryLater
			}
		}
		embeddingConsumerMessages.Inc("processed")
		observeEmbeddingIndexLag(e.EventTime, time.Now())
	}
	return consumer.ConsumeSuccess
}

// An empty successful protobuf repeated field decodes as nil. It represents an
// authoritative absence; a nil response or RPC error does not.
func authoritativePost(ctx context.Context, source visibility.PostsByIDs, id int64) (*contentservice.PostInfo, error) {
	if source == nil {
		return nil, fmt.Errorf("content authority is required")
	}
	response, err := source.GetPostsByIds(ctx, &contentservice.GetPostsByIdsReq{PostIds: []int64{id}})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("nil Content authority response")
	}
	for _, post := range response.Posts {
		if post != nil && post.Id == id && visibilityx.IsPublished(post.Status) {
			return post, nil
		}
	}
	return nil, nil
}
