package apply

import (
	"context"
	"time"

	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/store"
	"esx/pkg/event"

	"esx/pkg/logging"
)

// Ensurer 是审核平台的对账入口（RVW-041）。
type Ensurer interface {
	EnsureSubmitted(ctx context.Context, submission event.ReviewSubmittedEvent) (taskID int64, created bool, err error)
}

// ReconcileStore 是对账需要的存储操作。
type ReconcileStore interface {
	PendingAdsWithoutTask(ctx context.Context, now time.Time, limit int) ([]store.Ad, error)
	PendingAdvertisersWithoutTask(ctx context.Context, now time.Time, limit int) ([]store.Advertiser, error)
	SnapshotAt(ctx context.Context, adID, revision int64) (*store.Snapshot, error)
	AdvertiserByID(ctx context.Context, id int64) (*store.AdvertiserWithQualifications, error)
	RecordAdTask(ctx context.Context, adID, revision, taskID int64) error
	RecordAdvertiserTask(ctx context.Context, advertiserID, revision, taskID int64) error
	ExpireQualifications(ctx context.Context, now time.Time) ([]store.Qualification, error)
	MarkQualificationLapsed(ctx context.Context, q store.Qualification, now time.Time) ([]int64, error)
	ServingCandidates(ctx context.Context) ([]store.Ad, error)
}

// Reconciler 运行业务方的定时任务。
type Reconciler struct {
	Store  ReconcileStore
	Review Ensurer
	Server *Server
	Clock  func() time.Time
}

func (r *Reconciler) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// ReconcileSubmissions 找出送审超过 5 分钟仍未登记任务的对象，用同一快照幂等补送（RVW-041）。
func (r *Reconciler) ReconcileSubmissions(ctx context.Context) {
	now := r.now()
	ads, err := r.Store.PendingAdsWithoutTask(ctx, now, 50)
	if err != nil {
		logging.WithContext(ctx).Errorw("reconcile ads query failed", logging.Field("err", err.Error()))
	}
	for i := range ads {
		ad := &ads[i]
		snap, err := r.Store.SnapshotAt(ctx, ad.ID, ad.Revision)
		if err != nil {
			continue
		}
		purpose := event.ReviewPurposeInitial
		if ad.ReviewStatus == store.ReviewAppealing {
			purpose = event.ReviewPurposeAppeal
		}
		r.ensure(ctx, event.ReviewBizAdCreative, store.AdSubmission(ad, snap, purpose, now), func(taskID int64) error {
			return r.Store.RecordAdTask(ctx, ad.ID, ad.Revision, taskID)
		})
	}
	advertisers, err := r.Store.PendingAdvertisersWithoutTask(ctx, now, 50)
	if err != nil {
		logging.WithContext(ctx).Errorw("reconcile advertisers query failed", logging.Field("err", err.Error()))
	}
	for _, adv := range advertisers {
		full, err := r.Store.AdvertiserByID(ctx, adv.ID)
		if err != nil || full.Advertiser.Revision != adv.Revision {
			continue
		}
		r.ensure(ctx, event.ReviewBizAdvertiserQualification, store.AdvertiserSubmission(full, now), func(taskID int64) error {
			return r.Store.RecordAdvertiserTask(ctx, adv.ID, adv.Revision, taskID)
		})
	}
}

func (r *Reconciler) ensure(ctx context.Context, bizType string, sub event.ReviewSubmittedEvent, record func(int64) error) {
	taskID, created, err := r.Review.EnsureSubmitted(ctx, sub)
	if err != nil {
		metrics.Reconcile(bizType, "error")
		logging.WithContext(ctx).Errorw("ensure submitted failed", logging.Field("objectId", sub.ObjectID), logging.Field("err", err.Error()))
		return
	}
	outcome := "confirmed"
	if created {
		outcome = "repaired"
	}
	metrics.Reconcile(bizType, outcome)
	if err := record(taskID); err != nil {
		logging.WithContext(ctx).Errorw("record review task failed", logging.Field("objectId", sub.ObjectID), logging.Field("err", err.Error()))
	}
}

// ExpireQualifications 把到期资质标为 expired，并把依赖它的广告移出投放（ADS-002）。
func (r *Reconciler) ExpireQualifications(ctx context.Context) {
	now := r.now()
	expired, err := r.Store.ExpireQualifications(ctx, now)
	if err != nil {
		logging.WithContext(ctx).Errorw("expire qualifications failed", logging.Field("err", err.Error()))
	}
	for _, q := range expired {
		ids, err := r.Store.MarkQualificationLapsed(ctx, q, now)
		if err != nil {
			logging.WithContext(ctx).Errorw("mark qualification lapse failed", logging.Field("err", err.Error()))
			continue
		}
		for _, id := range ids {
			if _, err := r.Server.Refresh(ctx, id); err != nil {
				logging.WithContext(ctx).Errorw("refresh serving after lapse failed", logging.Field("adId", id), logging.Field("err", err.Error()))
			}
		}
	}
}

// RebuildIndex 周期性按权威状态校正投放索引，并重试失败的素材发布。
func (r *Reconciler) RebuildIndex(ctx context.Context) {
	ads, err := r.Store.ServingCandidates(ctx)
	if err != nil {
		logging.WithContext(ctx).Errorw("load serving candidates failed", logging.Field("err", err.Error()))
		return
	}
	for _, ad := range ads {
		if _, err := r.Server.Refresh(ctx, ad.ID); err != nil {
			logging.WithContext(ctx).Errorw("rebuild serving entry failed", logging.Field("adId", ad.ID), logging.Field("err", err.Error()))
		}
	}
}
