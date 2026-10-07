package mqs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/content/mq/cleanup/internal/store"
	"esx/app/content/mq/cleanup/internal/svc"
	"esx/pkg/event"
	"esx/pkg/mqx"

	"esx/pkg/logging"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
)

// countSyncTagExpression 订阅互动权威事件：like/unlike/favorite/unfavorite。
const countSyncTagExpression = "like || unlike || favorite || unfavorite"

// NewCountSyncConsumer 订阅 user-behavior-v2 上的互动动作，把 Interaction 的权威计数快照投影到内容表。
// CORE-032：公开计数允许最终一致，但必须在 30 秒内收敛；快照与互动状态经 outbox 同事务投递。
func NewCountSyncConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	cfg, err := svcCtx.Config.CountSyncConsumerConfig()
	if err != nil {
		return nil, err
	}
	c, err := mqx.NewConsumer(cfg)
	if err != nil {
		return nil, fmt.Errorf("count-sync-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeCountSyncBatch(ctx, svcCtx.CountSyncStore, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicUserBehaviorV2, countSyncTagExpression, handler); err != nil {
		return nil, fmt.Errorf("count-sync-consumer: subscribe %s: %w", mqx.TopicUserBehaviorV2, err)
	}
	return c, nil
}

// consumeCountSyncBatch 把点赞/收藏事件携带的权威计数快照投影到帖子与评论；投影按序号单调覆盖，
// 重复投递与乱序到达无需去重即可收敛。临时失败整批重试，永久失败确认丢弃。
func consumeCountSyncBatch(ctx context.Context, cs store.CountSyncStore, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, msg := range msgs {
		var behavior event.BehaviorEvent
		if err := json.Unmarshal(msg.Body, &behavior); err != nil {
			logging.WithContext(ctx).Errorw("count-sync-consumer: unmarshal failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("invalid")
			// 结构化损坏的事件无法恢复，重试也不会成功：计数后跳过。
			continue
		}
		if err := behavior.Validate(); err != nil {
			logging.WithContext(ctx).Errorw("count-sync-consumer: invalid event, skipping",
				logging.Field("msg_id", msg.MsgId), logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("invalid")
			continue
		}
		applied, err := cs.ApplyBehaviorCount(ctx, behavior)
		// 缺快照、目标类型未知等永久错误：重试无益，确认丢弃，计数由同一目标的下一次快照修正。
		if mqx.IsPermanentEvent(err) {
			logging.WithContext(ctx).Errorw("count-sync-consumer: permanent event skipped",
				logging.Field("msg_id", msg.MsgId), logging.Field("event_id", behavior.EventID),
				logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("invalid")
			continue
		}
		// 数据库等临时故障：整批稍后重试，已推进的投影会因序号守卫而成为空操作。
		if err != nil {
			logging.WithContext(ctx).Errorw("count-sync-consumer: apply count failed",
				logging.Field("msg_id", msg.MsgId), logging.Field("event_id", behavior.EventID),
				logging.Field("action", behavior.Action), logging.Field("err", err.Error()))
			countSyncConsumerMessages.Inc("retry")
			return consumer.ConsumeRetryLater
		}
		// 旧序号或重复投递：投影已包含该快照或更新的快照，确认即可。
		if !applied {
			countSyncConsumerMessages.Inc("stale")
			continue
		}
		countSyncConsumerMessages.Inc("processed")
		observeCountSyncLag(behavior.EventTime, time.Now())
		logging.WithContext(ctx).Infow("count-sync-consumer: count applied",
			logging.Field("event_id", behavior.EventID),
			logging.Field("action", behavior.Action),
			logging.Field("target_id", behavior.TargetID),
			logging.Field("target_type", behavior.TargetType),
			logging.Field("count_seq", snapshotSeq(behavior)))
	}
	return consumer.ConsumeSuccess
}

// snapshotSeq 返回事件携带的计数序号；无快照时为 0，仅用于日志。
func snapshotSeq(behavior event.BehaviorEvent) int64 {
	if behavior.CountSnapshot == nil {
		return 0
	}
	return behavior.CountSnapshot.Seq
}
