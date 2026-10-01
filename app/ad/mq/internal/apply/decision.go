package apply

import (
	"context"
	"time"

	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/store"
	"esx/pkg/event"

	logx "esx/pkg/logging"
)

// DecisionStore 是应用结论需要的存储操作。
type DecisionStore interface {
	ApplyAdDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (store.DecisionEffect, error)
	ApplyAdvertiserDecision(ctx context.Context, d event.ReviewDecidedEvent, now time.Time) (store.DecisionEffect, error)
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
		if !effect.Applied && d.Verdict != event.ReviewVerdictApprove && d.Purpose != event.ReviewPurposeQA {
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
		logx.WithContext(ctx).Infow("ad review decision applied",
			logx.Field("adId", d.ObjectID), logx.Field("revision", d.Revision),
			logx.Field("verdict", d.Verdict), logx.Field("applied", effect.Applied), logx.Field("servable", servable))
	case event.ReviewBizAdvertiserQualification:
		effect, err := a.Store.ApplyAdvertiserDecision(ctx, d, now)
		if err != nil {
			return err
		}
		metrics.Applied(d.BizType, result(effect.Applied))
	}
	return nil
}

func result(applied bool) string {
	if applied {
		return "applied"
	}
	return "stale"
}
