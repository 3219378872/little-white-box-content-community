package model

import (
	"context"
	"esx/pkg/idempotencyx"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// MediaCommandResult 是一次媒体创建命令的结果。
type MediaCommandResult struct {
	MediaID int64
	Created bool
}

// MediaCommandModel 负责媒体权威写入（媒体行 + 幂等键同事务）。
type MediaCommandModel interface {
	CreateMedia(ctx context.Context, media *Media, idem idempotencyx.IdempotencyRecord) (MediaCommandResult, error)
}

type mediaCommandModel struct {
	conn sqlx.SqlConn
}

func NewMediaCommandModel(conn sqlx.SqlConn) MediaCommandModel {
	return &mediaCommandModel{conn: conn}
}

// CreateMedia 在同事务内插入媒体行与幂等记录，避免重试产生重复资源（CORE-050）。
func (m *mediaCommandModel) CreateMedia(ctx context.Context, media *Media, idem idempotencyx.IdempotencyRecord) (MediaCommandResult, error) {
	if media == nil || m.conn == nil {
		return MediaCommandResult{}, fmt.Errorf("media command model is not configured")
	}
	var result MediaCommandResult
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		resourceID, created, err := idempotencyx.ResolveIdempotencySession(ctx, session, idem, media.Id, media.Id)
		if err != nil {
			return err
		}
		if !created {
			result = MediaCommandResult{MediaID: resourceID, Created: false}
			return nil
		}
		if _, err := session.ExecCtx(ctx, `INSERT INTO media
			(id, user_id, file_name, original_name, file_type, mime_type, url, thumbnail_url,
			 storage_type, bucket, object_key, file_size, width, height, duration, format, bit_rate, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			media.Id, media.UserId, media.FileName, media.OriginalName, media.FileType, media.MimeType,
			media.Url, media.ThumbnailUrl, media.StorageType, media.Bucket, media.ObjectKey,
			media.FileSize, media.Width, media.Height, media.Duration, media.Format, media.BitRate, media.Status,
		); err != nil {
			return err
		}
		result = MediaCommandResult{MediaID: media.Id, Created: true}
		return nil
	})
	return result, err
}
