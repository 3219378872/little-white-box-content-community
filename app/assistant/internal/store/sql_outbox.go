package store

import (
	"context"
	"database/sql"
	"strconv"
)

func (s *SQLStore) InsertOutbox(ctx context.Context, row Outbox) error {
	_, err := s.exec.ExecCtx(ctx, `INSERT INTO assistant_index_outbox (user_id, message_id, op, payload_json, published, created_at_ms)
		VALUES (?, ?, ?, ?, 0, ?)`, row.UserID, row.MessageID, row.Op, nullString(row.PayloadJSON), row.CreatedAtMs)
	return err
}

func (s *SQLStore) ListUnpublishedOutbox(ctx context.Context, limit int) ([]Outbox, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []struct {
		ID          int64          `db:"id"`
		UserID      int64          `db:"user_id"`
		MessageID   int64          `db:"message_id"`
		Op          string         `db:"op"`
		PayloadJSON sql.NullString `db:"payload_json"`
		Published   int64          `db:"published"`
		CreatedAtMs int64          `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, message_id, op, payload_json, published, created_at_ms
		FROM assistant_index_outbox WHERE published=0 ORDER BY id ASC LIMIT `+strconv.Itoa(limit)); err != nil {
		return nil, err
	}
	out := make([]Outbox, 0, len(rows))
	for _, row := range rows {
		out = append(out, Outbox{
			ID: row.ID, UserID: row.UserID, MessageID: row.MessageID, Op: row.Op, PayloadJSON: row.PayloadJSON.String,
			Published: row.Published == 1, CreatedAtMs: row.CreatedAtMs,
		})
	}
	return out, nil
}

func (s *SQLStore) MarkOutboxPublished(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.exec.ExecCtx(ctx, `UPDATE assistant_index_outbox SET published=1 WHERE id IN (`+placeholders(len(ids))+`)`, intsToAny(ids)...)
	return err
}
