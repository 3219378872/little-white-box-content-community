package logic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"

	logx "esx/pkg/logging"
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

	eventID := behaviorDedupKey(e)
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

// Redis is only a delivery optimization: the durable canonical ClickHouse view
// applies the same business boundary even if a write ACK or receipt is lost.
// Preserve event IDs for non-exposures and never mutate the original envelope.
func behaviorDedupKey(e event.BehaviorEvent) string {
	if e.Action != event.BehaviorActionExposure || e.TargetType != "post" {
		return e.EventIDString()
	}
	// A structured tuple cannot alias requests containing separators.
	key := strconv.Itoa(len(e.RequestID)) + ":" + e.RequestID + ":" + strconv.FormatInt(e.TargetID, 10)
	digest := sha256.Sum256([]byte(key))
	return "exposure:" + hex.EncodeToString(digest[:])
}
