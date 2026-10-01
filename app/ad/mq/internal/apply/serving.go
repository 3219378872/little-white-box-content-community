// Package apply 把审核结论落到广告状态与投放索引（DES-sponsored-ads「状态与投放资格」「传播」）。
package apply

import (
	"context"
	"errors"
	"fmt"
	"time"

	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/pkg/adpolicy"
)

// Publisher 把过审素材发布到公开内容寻址路径。
type Publisher interface {
	Publish(ctx context.Context, privateKey, sha string) error
}

// Store 是投放维护需要的存储操作。
type Store interface {
	AdByID(ctx context.Context, id int64) (*store.Ad, error)
	SnapshotAt(ctx context.Context, adID, revision int64) (*store.Snapshot, error)
	QualificationsOf(ctx context.Context, advertiserID int64) ([]store.Qualification, error)
	GetAsset(ctx context.Context, id int64) (*store.Asset, error)
	MarkPublished(ctx context.Context, adID, revision int64) error
}

// IndexWriter 是投放索引的写操作。
type IndexWriter interface {
	Upsert(ctx context.Context, market string, entry serving.Entry) error
	Remove(ctx context.Context, market string, adID int64) error
}

// Server 维护单个广告的投放索引条目。
type Server struct {
	Store     Store
	Publisher Publisher
	Index     IndexWriter
	Clock     func() time.Time
}

func (s *Server) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// Refresh 按当前权威状态放入或移出投放索引。可投放的条件：投放状态为 serving、存在过审快照、
// 受监管行业资质有效，且过审素材已发布（ADS-010、ADS-015）。素材发布失败时不进入索引，返回错误供重试。
func (s *Server) Refresh(ctx context.Context, adID int64) (bool, error) {
	ad, err := s.Store.AdByID(ctx, adID)
	if err != nil {
		return false, err
	}
	var snap *store.Snapshot
	if ad.ServingStatus == store.ServingServing && ad.ApprovedRevision > 0 {
		snap, err = s.Store.SnapshotAt(ctx, ad.ID, ad.ApprovedRevision)
		if err != nil {
			return false, err
		}
	}
	servable := snap != nil
	if servable && adpolicy.DispositionOf(snap.Market, snap.Industry).NeedsQualification {
		quals, err := s.Store.QualificationsOf(ctx, ad.AdvertiserID)
		if err != nil {
			return false, err
		}
		servable = store.ValidQualification(quals, snap.Market, snap.Industry, s.now().UnixMilli())
	}
	if servable && ad.PublishedRevision < ad.ApprovedRevision {
		if err := s.publish(ctx, ad, snap); err != nil {
			metrics.Degraded("assets", "publish")
			// 复制失败的广告不得留在索引中；移除失败与发布失败一并返回，由消息或定时重建重试。
			if removeErr := s.removeEverywhere(ctx, ad.ID, ""); removeErr != nil {
				return false, errors.Join(err, removeErr)
			}
			return false, err
		}
	}
	keep := ""
	if servable {
		keep = snap.Market
		if err := s.Index.Upsert(ctx, snap.Market, serving.Entry{
			AdID: ad.ID, Revision: ad.ApprovedRevision, AdvertiserID: ad.AdvertiserID, Industry: snap.Industry,
			StartMs: ad.StartMs, EndMs: ad.EndMs,
		}); err != nil {
			return false, err
		}
	}
	if err := s.removeEverywhere(ctx, ad.ID, keep); err != nil {
		return false, err
	}
	return servable, nil
}

func (s *Server) publish(ctx context.Context, ad *store.Ad, snap *store.Snapshot) error {
	for _, media := range snap.Media() {
		asset, err := s.Store.GetAsset(ctx, media.MediaID)
		if err != nil {
			return fmt.Errorf("apply: load asset %d: %w", media.MediaID, err)
		}
		if err := s.Publisher.Publish(ctx, asset.ObjectKey, asset.SHA256); err != nil {
			return err
		}
	}
	return s.Store.MarkPublished(ctx, ad.ID, ad.ApprovedRevision)
}

func (s *Server) removeEverywhere(ctx context.Context, adID int64, keep string) error {
	for _, market := range adpolicy.Markets {
		if market.Code == keep {
			continue
		}
		if err := s.Index.Remove(ctx, market.Code, adID); err != nil {
			return err
		}
	}
	return nil
}
