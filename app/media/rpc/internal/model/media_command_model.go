package model

import (
	"context"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// MediaCommandResult 是一次媒体创建命令的结果。
type MediaCommandResult struct {
	MediaID int64
	Created bool
}

// OutboxEnqueuer 在业务事务内写入事务发件箱（outbox）。
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

// MediaCommandModel 负责媒体权威写入（媒体行 + 幂等键 / 软删 + outbox 同事务）。
type MediaCommandModel interface {
	CreateMedia(ctx context.Context, media *Media, idem idempotencyx.IdempotencyRecord) (MediaCommandResult, error)
	// SoftDelete 条件软删（status 1→0）并在同一事务内写入 outbox 事件；
	// 行已被删除（rowsAffected=0）时不写事件，保证幂等且不产生重复清理。
	SoftDelete(ctx context.Context, mediaID int64, event outboxx.Event) error
	// EnqueueObjectCleanup durably schedules deletion after an immediate
	// compensation attempt failed. No media row exists for upload failures.
	EnqueueObjectCleanup(ctx context.Context, event outboxx.Event) error
}

func (m *mediaCommandModel) EnqueueObjectCleanup(ctx context.Context, event outboxx.Event) error {
	if m.conn == nil || m.outbox == nil {
		return fmt.Errorf("media command model is not configured")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		return m.outbox.Enqueue(ctx, session, event)
	})
}

type mediaCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

func NewMediaCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) MediaCommandModel {
	return &mediaCommandModel{conn: conn, outbox: outbox}
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

// SoftDelete 条件软删并在同事务投递 outbox 事件（架构约定：权威业务事务通过
// outbox 同事务投递，避免提交后进程崩溃导致事件丢失、S3 对象成为孤儿）。
// 事件只在确实发生删除（rowsAffected>0）且存在清理需要（event.ID>0）时写入。
func (m *mediaCommandModel) SoftDelete(ctx context.Context, mediaID int64, event outboxx.Event) error {
	if mediaID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("media command model is not configured")
	}
	if event.ID > 0 {
		if err := event.Validate(); err != nil {
			return err
		}
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			`UPDATE media SET status = 0 WHERE id = ? AND status = 1`, mediaID)
		if err != nil {
			return err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			// 无法确认删除是否发生时不静默跳过事件投递，避免事件丢失。
			return err
		}
		if rowsAffected == 0 {
			// 并发重复删除：不投递事件，保持幂等。
			return nil
		}
		if event.ID == 0 {
			// 无 S3 对象可清理：仅软删。
			return nil
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}
