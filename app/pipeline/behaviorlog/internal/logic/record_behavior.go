package logic

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"time"

	"esx/pkg/event"
	"mqx"
	"util"

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
		logx.WithContext(ctx).Errorw("behavior-log: dedup check failed, falling through",
			logx.Field("event_id", e.EventID), logx.Field("err", err.Error()))
	} else if dup {
		logx.WithContext(ctx).Infow("behavior-log: duplicate event skipped",
			logx.Field("event_id", e.EventID))
		return nil
	}

	if err := r.store.Insert(ctx, e); err != nil {
		return fmt.Errorf("record behavior event: %w", err)
	}

	if err := r.dedup.MarkProcessed(ctx, eventID); err != nil {
		logx.WithContext(ctx).Errorw("behavior-log: mark processed failed",
			logx.Field("event_id", e.EventID), logx.Field("err", err.Error()))
	}

	logx.WithContext(ctx).Infow("behavior-log: event recorded",
		logx.Field("event_id", e.EventID), logx.Field("user_id", e.UserID),
		logx.Field("action", e.Action))

	return nil
}

func normalizeBehaviorEvent(e event.BehaviorEvent, meta MessageMeta) (event.BehaviorEvent, error) {
	if e.EventID == 0 {
		id, err := eventIDFromMeta(meta)
		if err != nil {
			return event.BehaviorEvent{}, err
		}
		e.EventID = id
	}

	if e.EventTime == 0 {
		e.EventTime = eventTimeFromMeta(meta)
	}

	return e, nil
}

func eventIDFromMeta(meta MessageMeta) (int64, error) {
	stableID := meta.MsgID
	if stableID == "" {
		stableID = meta.OffsetMsgID
	}
	if stableID != "" {
		h := fnv.New64a()
		if _, err := h.Write([]byte(stableID)); err != nil {
			return 0, err
		}
		id := int64(h.Sum64() & math.MaxInt64)
		if id > 0 {
			return id, nil
		}
	}

	return util.NextID()
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
