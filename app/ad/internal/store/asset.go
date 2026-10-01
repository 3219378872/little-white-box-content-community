package store

import (
	"context"
	"strconv"
	"time"

	sqlx "esx/pkg/sqlstore"
)

// CreateAsset 登记已写入私有存储的素材或证件；同一幂等键返回首次结果。
func (s *Store) CreateAsset(ctx context.Context, asset Asset, idempotencyKey string, now time.Time) (*Asset, error) {
	var out Asset
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		recordID, err := s.nextID()
		if err != nil {
			return err
		}
		resourceID, created, err := resolve(ctx, session, "ad:asset:upload", asset.UserID, idempotencyKey, recordID, asset.ID,
			asset.Kind, asset.SHA256, strconv.FormatInt(asset.SizeBytes, 10))
		if err != nil {
			return err
		}
		if !created {
			return session.QueryRowCtx(ctx, &out, `SELECT `+assetColumns+` FROM ad_asset WHERE id = ?`, resourceID)
		}
		asset.CreatedAtMs = now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `INSERT INTO ad_asset (`+assetColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			asset.ID, asset.UserID, asset.Kind, asset.SHA256, asset.MimeType, asset.SizeBytes, asset.ObjectKey,
			asset.CreatedAtMs); err != nil {
			return err
		}
		out = asset
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAsset 读取资产元数据。
func (s *Store) GetAsset(ctx context.Context, id int64) (*Asset, error) {
	var asset Asset
	if err := s.conn.QueryRowCtx(ctx, &asset, `SELECT `+assetColumns+` FROM ad_asset WHERE id = ?`, id); err != nil {
		return nil, notFound(err)
	}
	return &asset, nil
}

// NewAssetID 为待上传资产分配 ID（对象键在写库前确定）。
func (s *Store) NewAssetID() (int64, error) { return s.nextID() }
