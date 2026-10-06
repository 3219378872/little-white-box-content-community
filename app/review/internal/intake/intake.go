// Package intake 是送审入口：事件消费与 EnsureSubmitted 对账共用同一路径（RVW-001、RVW-041）。
package intake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"esx/app/review/internal/metrics"
	"esx/app/review/internal/policy"
	"esx/app/review/internal/snapshot"
	"esx/app/review/internal/store"
	"esx/pkg/event"
)

// Ingester 冻结快照并幂等建任务。
type Ingester struct {
	Store  *store.Store
	Policy *policy.Policy
	Clock  func() time.Time
}

// ErrInvalidSubmission 表示载荷无法解析或不满足契约，属于永久错误。
type ErrInvalidSubmission struct{ cause error }

// Error 返回带原因的错误描述。
func (e ErrInvalidSubmission) Error() string { return "intake: invalid submission: " + e.cause.Error() }

// Unwrap 暴露底层原因，便于 errors.Is/As 判断。
func (e ErrInvalidSubmission) Unwrap() error { return e.cause }

// Decode 解析并校验送审载荷。
func Decode(raw []byte) (event.ReviewSubmittedEvent, error) {
	var sub event.ReviewSubmittedEvent
	if err := json.Unmarshal(raw, &sub); err != nil {
		return sub, ErrInvalidSubmission{cause: err}
	}
	if err := sub.Validate(); err != nil {
		return sub, ErrInvalidSubmission{cause: err}
	}
	return sub, nil
}

// Ingest 写入快照与任务。
func (i *Ingester) Ingest(ctx context.Context, sub event.ReviewSubmittedEvent) (store.IngestResult, error) {
	if err := sub.Validate(); err != nil {
		metrics.Ingested(sub.BizType, "invalid")
		return store.IngestResult{}, ErrInvalidSubmission{cause: err}
	}
	frozen, err := snapshot.Freeze(sub.BizType, sub.Snapshot)
	if err != nil {
		return store.IngestResult{}, fmt.Errorf("intake: freeze: %w", err)
	}
	now := time.Now()
	if i.Clock != nil {
		now = i.Clock()
	}
	result, err := i.Store.Ingest(ctx, sub, frozen, i.Policy.Version, i.Policy.DeadlineMs(sub.SubmittedAt), now)
	if err != nil {
		return store.IngestResult{}, err
	}
	switch {
	case !result.Created:
		metrics.Ingested(sub.BizType, "duplicate")
	case result.Task.Status == store.StatusSuperseded:
		metrics.Ingested(sub.BizType, "superseded")
	default:
		metrics.Ingested(sub.BizType, "created")
	}
	return result, nil
}
