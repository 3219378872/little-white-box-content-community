package apply

import (
	"context"
	"time"

	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/store"

	logx "esx/pkg/logging"
)

// Generation 是审核平台给出的回扫代次（ADS-031）。
type Generation struct {
	Ready            bool
	Value            string
	EffectiveSinceMs int64
}

// GenerationSource 读取当前回扫代次；生产经 review-rpc.GetRescanGeneration。
type GenerationSource interface {
	RescanGeneration(ctx context.Context) (Generation, error)
}

// RescanStore 是回扫需要的存储操作。
type RescanStore interface {
	RescanCandidates(ctx context.Context, generation string, limit int) ([]store.Ad, error)
	MarkRescanned(ctx context.Context, adID, approvedRevision int64, generation string) error
	SubmitRescan(ctx context.Context, adID, approvedRevision int64, generation string, now time.Time) (bool, error)
}

// RescanBatch 与 RescanBatchesPerTick 限制每轮处理量，避免一次代次变化占满 outbox 与审核队列。
const (
	RescanBatch          = 100
	RescanBatchesPerTick = 5
)

// Rescanner 在政策版本或生效种子库变化后，把在投广告的过审快照送回扫（ADS-031）。
// 审核平台不了解业务对象全集，因此由业务方枚举（DES-sponsored-ads「投后」）。
type Rescanner struct {
	Store  RescanStore
	Source GenerationSource
	Clock  func() time.Time
}

func (r *Rescanner) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// Run 处理一轮：结论时间早于代次生效时间的广告送回扫；之后过审的广告已按该代次审核，只记录代次。
func (r *Rescanner) Run(ctx context.Context) {
	gen, err := r.Source.RescanGeneration(ctx)
	if err != nil {
		logx.WithContext(ctx).Errorw("load rescan generation failed", logx.Field("err", err.Error()))
		return
	}
	if !gen.Ready || gen.Value == "" {
		return
	}
	for range RescanBatchesPerTick {
		ads, err := r.Store.RescanCandidates(ctx, gen.Value, RescanBatch)
		if err != nil {
			logx.WithContext(ctx).Errorw("load rescan candidates failed", logx.Field("err", err.Error()))
			return
		}
		progressed := false
		for _, ad := range ads {
			if r.handle(ctx, ad, gen) {
				progressed = true
			}
		}
		if len(ads) < RescanBatch || !progressed {
			return
		}
	}
}

func (r *Rescanner) handle(ctx context.Context, ad store.Ad, gen Generation) bool {
	if ad.ApprovedAtMs >= gen.EffectiveSinceMs {
		if err := r.Store.MarkRescanned(ctx, ad.ID, ad.ApprovedRevision, gen.Value); err != nil {
			logx.WithContext(ctx).Errorw("mark rescanned failed", logx.Field("adId", ad.ID), logx.Field("err", err.Error()))
			return false
		}
		metrics.Rescan("current")
		return true
	}
	submitted, err := r.Store.SubmitRescan(ctx, ad.ID, ad.ApprovedRevision, gen.Value, r.now())
	if err != nil {
		metrics.Rescan("error")
		logx.WithContext(ctx).Errorw("submit rescan failed", logx.Field("adId", ad.ID), logx.Field("err", err.Error()))
		return false
	}
	if submitted {
		metrics.Rescan("submitted")
	} else {
		metrics.Rescan("skipped")
	}
	return true
}
