package logic

import (
	"context"
	"fmt"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/logx"
)

type Deduper interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
}

type BehaviorProcessor interface {
	Process(ctx context.Context, e event.BehaviorEvent, meta MessageMeta) error
}

type MessageMeta struct {
	MsgID          string
	OffsetMsgID    string
	StoreTimestamp int64
	BornTimestamp  int64
}

type Recorder struct {
	store interface {
		Insert(ctx context.Context, e event.BehaviorEvent) error
	}
	dedup Deduper
}

func NewRecorder(s interface {
	Insert(ctx context.Context, e event.BehaviorEvent) error
}, d Deduper) *Recorder {
	return &Recorder{store: s, dedup: d}
}

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
		logx.WithContext(ctx).Infow("behavior-log: duplicate event skipped",
			logx.Field("event_id", e.EventID))
		return nil
	}

	if err := r.store.Insert(ctx, e); err != nil {
		return fmt.Errorf("record behavior event: %w", err)
	}

	if err := r.dedup.MarkProcessed(ctx, eventID); err != nil {
		return fmt.Errorf("behavior-log: mark processed: %w", err)
	}

	logx.WithContext(ctx).Infow("behavior-log: event recorded",
		logx.Field("event_id", e.EventID), logx.Field("user_id", e.UserID),
		logx.Field("action", e.Action))

	return nil
}

func normalizeBehaviorEvent(e event.BehaviorEvent, meta MessageMeta) (event.BehaviorEvent, error) {
	if e.EventID == 0 && e.ClientEventID != "" {
		e.EventID = event.DeterministicBehaviorEventID(e.ClientEventID)
	}
	if e.ReceivedAt == 0 {
		e.ReceivedAt = eventTimeFromMeta(meta)
	}
	return e, nil
}

func eventTimeFromMeta(meta MessageMeta) int64 {
	if meta.StoreTimestamp > 0 {
		return meta.StoreTimestamp
	}
	if meta.BornTimestamp > 0 {
		return meta.BornTimestamp
	}
	return time.Now().UnixMilli()
}
