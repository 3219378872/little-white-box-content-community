package model

import (
	"context"
	"database/sql"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"
	"fmt"
	"time"

	sqlx "esx/pkg/sqlstore"
)

// MediaCommitOutcome describes whether uploaded objects may have a durable owner.
// The zero value is deliberately unknown: an unclassified failure must not delete data.
type MediaCommitOutcome uint8

const (
	MediaCommitUnknown MediaCommitOutcome = iota
	MediaNotCommitted
	MediaCommitted
)

const mediaCommitReconcileTimeout = 2 * time.Second

// MediaCommandResult 是一次媒体创建命令的结果。
type MediaCommandResult struct {
	MediaID int64
	Created bool
	Outcome MediaCommitOutcome
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
	SoftDelete(ctx context.Context, mediaID int64, events ...outboxx.Event) error
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
		return MediaCommandResult{Outcome: MediaNotCommitted}, fmt.Errorf("media command model is not configured")
	}
	// A callback error cannot issue COMMIT. Once the callback succeeds, a
	// transaction error is ambiguous (including cancellation before its ACK).
	result := MediaCommandResult{Outcome: MediaNotCommitted}
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		resourceID, created, err := idempotencyx.ResolveIdempotencySession(ctx, session, idem, media.Id, media.Id)
		if err != nil {
			return err
		}
		if !created {
			result = MediaCommandResult{MediaID: resourceID, Created: false, Outcome: MediaCommitUnknown}
			return nil
		}
		if _, err := session.ExecCtx(ctx, `INSERT INTO media
			(id, user_id, file_name, original_name, file_type, mime_type, url, thumbnail_url,
			 storage_type, bucket, object_key, thumbnail_object_key, file_size, width, height, duration, format, bit_rate, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			media.Id, media.UserId, media.FileName, media.OriginalName, media.FileType, media.MimeType,
			media.Url, media.ThumbnailUrl, media.StorageType, media.Bucket, media.ObjectKey, media.ThumbnailObjectKey,
			media.FileSize, media.Width, media.Height, media.Duration, media.Format, media.BitRate, media.Status,
		); err != nil {
			return err
		}
		result = MediaCommandResult{MediaID: media.Id, Created: true, Outcome: MediaCommitUnknown}
		return nil
	})
	if err == nil {
		result.Outcome = MediaCommitted
	} else if result.Outcome == MediaCommitUnknown && m.reconcileCreate(ctx, media, idem, result) {
		result.Outcome = MediaCommitted
		err = nil
	}
	return result, err
}

// reconcileCreate only upgrades positive proof of commit. A missing row, failed
// lookup or mismatched owner is NOT evidence of rollback. Use the command DB
// directly (the same authoritative writer, no cache/replica) and detach only the
// cancellation/deadline, keeping request values for tracing with a bounded budget.
func (m *mediaCommandModel) reconcileCreate(ctx context.Context, media *Media, idem idempotencyx.IdempotencyRecord, result MediaCommandResult) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaCommitReconcileTimeout)
	defer cancel()
	var owner struct {
		ID                 int64          `db:"id"`
		UserID             int64          `db:"user_id"`
		ObjectKey          sql.NullString `db:"object_key"`
		ThumbnailObjectKey sql.NullString `db:"thumbnail_object_key"`
		Status             int64          `db:"status"`
	}
	query := `SELECT m.id, m.user_id, m.object_key, m.thumbnail_object_key, m.status FROM media m`
	args := []any{result.MediaID}
	if idem.Key != "" {
		query += " JOIN `idempotency` i ON i.resource_id = m.id" +
			" WHERE m.id = ? AND i.scope = ? AND i.user_id = ? AND i.`key` = ? AND i.command_hash = ?"
		args = append(args, idem.Scope, idem.UserID, idem.Key, idem.CommandHash)
	} else {
		query += " WHERE m.id = ?"
	}
	query += " LIMIT 1"
	if err := m.conn.QueryRowCtx(ctx, &owner, query, args...); err != nil {
		return false
	}
	if owner.ID != result.MediaID || owner.UserID != media.UserId || owner.Status != 1 {
		return false
	}
	if result.Created {
		return owner.ID == media.Id && owner.ObjectKey == media.ObjectKey && owner.ThumbnailObjectKey == media.ThumbnailObjectKey
	}
	// A confirmed duplicate may discard only this attempt's fresh keys. Never
	// interpret a key collision or inconsistent identity as permission to delete.
	if owner.ID == media.Id {
		return false
	}
	for _, key := range []sql.NullString{media.ObjectKey, media.ThumbnailObjectKey} {
		if key.Valid && key.String != "" && (key == owner.ObjectKey || key == owner.ThumbnailObjectKey) {
			return false
		}
	}
	return true
}

// SoftDelete 条件软删并在同事务投递 outbox 事件（架构约定：权威业务事务通过
// outbox 同事务投递，避免提交后进程崩溃导致事件丢失、S3 对象成为孤儿）。
// 事件只在确实发生删除（rowsAffected>0）且存在清理需要（event.ID>0）时写入。
func (m *mediaCommandModel) SoftDelete(ctx context.Context, mediaID int64, events ...outboxx.Event) error {
	if mediaID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("media command model is not configured")
	}
	for _, event := range events {
		if event.ID > 0 {
			if err := event.Validate(); err != nil {
				return err
			}
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
		for _, event := range events {
			if event.ID == 0 {
				continue
			}
			if err := m.outbox.Enqueue(ctx, session, event); err != nil {
				return err
			}
		}
		return nil
	})
}
