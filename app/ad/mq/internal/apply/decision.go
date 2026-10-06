package apply

import (
	"context"
	"time"

	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/store"
	"esx/pkg/event"

	"esx/pkg/logging"
)

// DecisionStore 是应用结论需要的存储操作。
type DecisionStore interface {
	ApplyAdDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (store.DecisionEffect, error)
	ApplyAdvertiserDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (store.DecisionEffect, error)
	CloseReportBatch(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) error
}

// Applier 应用 review-decided 事件。
type Applier struct {
	Store  DecisionStore
	Server *Server
	Clock  func() time.Time
}

// Apply 按 revision CAS 应用结论；旧 revision 与重复结论影响 0 行，只记指标（RVW-040）。
// 投放变化立即同步索引（ADS-032），失败返回错误由消息重试。
func (a *Applier) Apply(ctx context.Context, d event.ReviewDecidedEvent) error {
	now := time.Now()
	if a.Clock != nil {
		now = a.Clock()
	}
	switch d.BizType {
	case event.ReviewBizAdCreative:
		effect, err := a.Store.ApplyAdDecision(ctx, d, now)
		if err != nil {
			return err
		}
		metrics.Applied(d.BizType, result(effect.Applied))
		if err := a.Store.CloseReportBatch(ctx, d, now); err != nil {
			return err
		}
		// 旧 revision 的送审与申诉拒绝不影响投放；其余结论（含投后任务与暂停结论）都按权威状态刷新索引。
		if !effect.Applied && d.Verdict == event.ReviewVerdictReject &&
			(d.Purpose == event.ReviewPurposeInitial || d.Purpose == event.ReviewPurposeAppeal) {
			return nil
		}
		// 重复投递时仍刷新索引，保证首轮发布失败后可经消息重试补齐。
		servable, err := a.Server.Refresh(ctx, d.ObjectID)
		if err != nil {
			return err
		}
		action := "remove"
		if servable {
			action = "serve"
		}
		metrics.Propagation(action, d.DecidedAt, a.Server.now())
		logging.WithContext(ctx).Infow("ad review decision applied",
			logging.Field("adId", d.ObjectID), logging.Field("revision", d.Revision),
			logging.Field("verdict", d.Verdict), logging.Field("purpose", d.Purpose), logging.Field("interim", d.Interim),
			logging.Field("applied", effect.Applied), logging.Field("servable", servable))
	case event.ReviewBizAdvertiserQualification:
		effect, err := a.Store.ApplyAdvertiserDecision(ctx, d, now)
		if err != nil {
			return err
		}
		metrics.Applied(d.BizType, result(effect.Applied))
	}
	return nil
}

// result 把结论是否生效映射为指标标签；未生效说明结论针对的版本已过期。
func result(applied bool) string {
	if applied {
		return "applied"
	}
	return "stale"
}
