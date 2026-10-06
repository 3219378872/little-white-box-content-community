package logic

import (
	"context"
	"fmt"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"

	"esx/pkg/logging"
)

// Deduper 判断并记录已处理的事件。
type Deduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

// BehaviorProcessor 处理一条行为事件；返回 mqx 永久错误时消息进入死信，其他错误触发重试。
type BehaviorProcessor interface {
	Process(ctx context.Context, e event.BehaviorEvent, meta MessageMeta) error
}

// MessageMeta 是来自 MQ 的消息元数据，用于补全缺失的接收时间。
type MessageMeta struct {
	MsgID          string
	OffsetMsgID    string
	StoreTimestamp int64
	BornTimestamp  int64
}

// Recorder 把行为事件去重后写入存储。
type Recorder struct {
	store interface {
		Insert(ctx context.Context, e event.BehaviorEvent) error
	}
	dedup Deduper
}

// NewRecorder 创建行为事件记录器。
func NewRecorder(s interface {
	Insert(ctx context.Context, e event.BehaviorEvent) error
}, d Deduper) *Recorder {
	return &Recorder{store: s, dedup: d}
}

// Process 补全并校验事件，跳过已处理的事件后落库，再写去重标记。落库成功但标记失败时消息会重试，
// 重复写入由 behavior_events 按 event_id 去重收敛。
func (r *Recorder) Process(ctx context.Context, e event.BehaviorEvent, meta MessageMeta) error {
	var err error
	e, err = normalizeBehaviorEvent(e, meta)
	if err != nil {
		return err
	}

	if err := e.Validate(); err != nil {
		return mqx.ErrPermanentEvent(fmt.Sprintf("validate behavior event: %v", err))
	}

	eventID := e.EventIDString()
	dup, err := r.dedup.IsDuplicate(ctx, eventID)
	if err != nil {
		return fmt.Errorf("behavior-log: dedup check: %w", err)
	}
	if dup {
		logging.WithContext(ctx).Infow("behavior-log: duplicate event skipped",
			logging.Field("event_id", e.EventID))
		return nil
	}

	if err := r.store.Insert(ctx, e); err != nil {
		return fmt.Errorf("record behavior event: %w", err)
	}

	if err := r.dedup.MarkProcessed(ctx, eventID); err != nil {
		return fmt.Errorf("behavior-log: mark processed: %w", err)
	}

	logging.WithContext(ctx).Infow("behavior-log: event recorded",
		logging.Field("event_id", e.EventID), logging.Field("user_id", e.UserID),
		logging.Field("action", e.Action))

	return nil
}

// normalizeBehaviorEvent 兼容旧生产者：缺 EventID 时由 client_event_id 派生，缺接收时间时取消息时间。
func normalizeBehaviorEvent(e event.BehaviorEvent, meta MessageMeta) (event.BehaviorEvent, error) {
	if e.EventID == 0 && e.ClientEventID != "" {
		e.EventID = event.DeterministicBehaviorEventID(e.ClientEventID)
	}
	if e.ReceivedAt == 0 {
		e.ReceivedAt = eventTimeFromMeta(meta)
	}
	return e, nil
}

// eventTimeFromMeta 按 Broker 存储时间、生产时间、当前时间的顺序取接收时间。
func eventTimeFromMeta(meta MessageMeta) int64 {
	if meta.StoreTimestamp > 0 {
		return meta.StoreTimestamp
	}
	if meta.BornTimestamp > 0 {
		return meta.BornTimestamp
	}
	return time.Now().UnixMilli()
}
