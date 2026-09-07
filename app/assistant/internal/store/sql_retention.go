package store

import (
	"context"
	"fmt"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// PurgeExpiredMessages removes one bounded batch of expired Assistant messages.
// Index cleanup rows are committed in the same transaction as the authoritative
// deletes so an expired message cannot remain searchable after a partial purge.
func (s *SQLStore) PurgeExpiredMessages(ctx context.Context, cutoffMs int64, batchSize int) (int, error) {
	if s.conn == nil {
		return 0, fmt.Errorf("purge expired messages requires SQL connection")
	}
	batchSize = boundedPurgeBatchSize(batchSize)
	deleted := 0
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		var rows []struct {
			ID     int64 `db:"id"`
			UserID int64 `db:"user_id"`
		}
		if err := session.QueryRowsCtx(ctx, &rows, `SELECT id, user_id FROM assistant_message
			WHERE created_at_ms < ? ORDER BY created_at_ms ASC, id ASC LIMIT ? FOR UPDATE`, cutoffMs, batchSize); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		args := intsToAny(ids)
		if _, err := session.ExecCtx(ctx, `DELETE FROM assistant_index_outbox WHERE message_id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
			return err
		}
		nowMs := NowMs()
		for _, row := range rows {
			if _, err := session.ExecCtx(ctx, `INSERT INTO assistant_index_outbox
				(user_id, message_id, op, payload_json, published, created_at_ms)
				VALUES (?, ?, ?, NULL, 0, ?)`, row.UserID, row.ID, IndexOpDelete, nowMs); err != nil {
				return err
			}
		}
		if _, err := session.ExecCtx(ctx, `UPDATE assistant_thread
			SET unread_count=0, last_message_id=0, last_message_preview='', last_message_at_ms=0
			WHERE last_message_id IN (`+placeholders(len(ids))+`)`, args...); err != nil {
			return err
		}
		result, err := session.ExecCtx(ctx, `DELETE FROM assistant_message WHERE id IN (`+placeholders(len(ids))+`)`, args...)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		deleted = int(affected)
		return nil
	})
	return deleted, err
}

func (s *SQLStore) PurgeExpiredWatchHits(ctx context.Context, cutoffMs int64, batchSize int) (int, error) {
	return s.purgeExpiredRows(ctx, `DELETE FROM watch_hit WHERE id IN (
		SELECT id FROM (SELECT id FROM watch_hit WHERE created_at_ms < ? ORDER BY id ASC LIMIT ?) expired
	)`, cutoffMs, boundedPurgeBatchSize(batchSize))
}

func (s *SQLStore) PurgeExpiredWatchExecutions(ctx context.Context, cutoffMs int64, batchSize int) (int, error) {
	return s.purgeExpiredRows(ctx, `DELETE FROM watch_execution WHERE id IN (
		SELECT id FROM (SELECT id FROM watch_execution WHERE created_at < FROM_UNIXTIME(?) ORDER BY id ASC LIMIT ?) expired
	)`, cutoffMs/1000, boundedPurgeBatchSize(batchSize))
}

func (s *SQLStore) purgeExpiredRows(ctx context.Context, query string, cutoff int64, batchSize int) (int, error) {
	result, err := s.exec.ExecCtx(ctx, query, cutoff, batchSize)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

func boundedPurgeBatchSize(size int) int {
	if size <= 0 {
		return 500
	}
	if size > 1000 {
		return 1000
	}
	return size
}
