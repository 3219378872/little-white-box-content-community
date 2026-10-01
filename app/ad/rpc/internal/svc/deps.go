package svc

import (
	"context"
	"time"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/store"
)

// Store 是 logic 用到的广告存储能力；生产使用 *store.Store，单测替换为内存实现。
type Store interface {
	ApplyAdvertiser(ctx context.Context, in store.AdvertiserInput, now time.Time) (*store.AdvertiserWithQualifications, error)
	GetAdvertiserByUser(ctx context.Context, userID int64) (*store.AdvertiserWithQualifications, error)
	AddQualification(ctx context.Context, in store.QualificationInput, now time.Time) (*store.AdvertiserWithQualifications, error)
	CreateAd(ctx context.Context, userID int64, in store.AdInput, idempotencyKey string, now time.Time) (*store.AdWithSnapshots, error)
	UpdateAd(ctx context.Context, userID, adID, expectedRevision int64, in store.AdInput, idempotencyKey string, now time.Time) (*store.AdWithSnapshots, error)
	GetAd(ctx context.Context, userID, adID int64) (*store.AdWithSnapshots, error)
	ListAds(ctx context.Context, userID, cursor int64, limit int) ([]store.AdWithSnapshots, int64, bool, error)
	SnapshotAt(ctx context.Context, adID, revision int64) (*store.Snapshot, error)
	QualificationsOf(ctx context.Context, advertiserID int64) ([]store.Qualification, error)
	NewAssetID() (int64, error)
	CreateAsset(ctx context.Context, asset store.Asset, idempotencyKey string, now time.Time) (*store.Asset, error)
	GetAsset(ctx context.Context, id int64) (*store.Asset, error)
}

// AssetStorage 是私有素材读写能力；生产使用 *assets.Storage。
type AssetStorage interface {
	PutPrivate(ctx context.Context, key string, content []byte, mime string) error
	GetPrivate(ctx context.Context, key string) ([]byte, error)
	DeletePrivate(ctx context.Context, key string) error
}

var (
	_ Store        = (*store.Store)(nil)
	_ AssetStorage = (*assets.Storage)(nil)
)
