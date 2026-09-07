package store

import (
	"context"
	"database/sql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"strconv"
)

func (s *SQLStore) InsertMessage(ctx context.Context, msg Message) (Message, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO assistant_message
		(user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.UserID, msg.SessionID, msg.RunID, msg.Role, msg.Kind, msg.Content, nullBytes(msg.APIContent),
		boolToInt(msg.Visible), boolToInt(msg.Unread), boolToInt(msg.Compacted), msg.ChangeID, nullInt(msg.DeletedAtMs), msg.CreatedAtMs)
	if err != nil {
		return Message{}, err
	}
	id, _ := res.LastInsertId()
	msg.ID = id
	return msg, nil
}

func (s *SQLStore) GetMessage(ctx context.Context, userID, id int64) (*Message, error) {
	rows, err := s.scanMessages(ctx, `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sqlx.ErrNotFound
	}
	return &rows[0], nil
}

func (s *SQLStore) ListMessages(ctx context.Context, userID, sessionID, beforeID, afterID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 101 {
		limit = 50
	}
	query := `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE user_id=? AND deleted_at_ms IS NULL AND visible=1`
	args := []any{userID}
	if sessionID > 0 {
		query += ` AND session_id=?`
		args = append(args, sessionID)
	}
	if afterID > 0 {
		query += ` AND id>?`
		args = append(args, afterID)
		query += ` ORDER BY id ASC LIMIT ` + strconv.Itoa(limit)
		return s.scanMessages(ctx, query, args...)
	}
	if beforeID > 0 {
		query += ` AND id<?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ` + strconv.Itoa(limit)
	rows, err := s.scanMessages(ctx, query, args...)
	reverseMessages(rows)
	return rows, err
}

func (s *SQLStore) ListSessionMessages(ctx context.Context, userID, sessionID int64, includeHidden bool) ([]Message, error) {
	query := `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE user_id=? AND session_id=? AND deleted_at_ms IS NULL`
	if !includeHidden {
		query += ` AND visible=1`
	}
	query += ` ORDER BY id ASC`
	return s.scanMessages(ctx, query, userID, sessionID)
}

func (s *SQLStore) GetMessagesByIDs(ctx context.Context, userID int64, ids []int64) ([]Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, change_id, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE user_id=? AND id IN (` + placeholders(len(ids)) + `)`
	args := append([]any{userID}, intsToAny(ids)...)
	return s.scanMessages(ctx, query, args...)
}

func (s *SQLStore) scanMessages(ctx context.Context, query string, args ...any) ([]Message, error) {
	var rows []struct {
		ID          int64         `db:"id"`
		UserID      int64         `db:"user_id"`
		SessionID   int64         `db:"session_id"`
		RunID       int64         `db:"run_id"`
		Role        string        `db:"role"`
		Kind        string        `db:"kind"`
		Content     string        `db:"content"`
		APIContent  []byte        `db:"api_content"`
		Visible     int64         `db:"visible"`
		Unread      int64         `db:"unread"`
		Compacted   int64         `db:"compacted"`
		ChangeID    int64         `db:"change_id"`
		DeletedAtMs sql.NullInt64 `db:"deleted_at_ms"`
		CreatedAtMs int64         `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, Message{
			ID: row.ID, UserID: row.UserID, SessionID: row.SessionID, RunID: row.RunID, Role: row.Role, Kind: row.Kind,
			Content: row.Content, APIContent: row.APIContent, Visible: row.Visible == 1, Unread: row.Unread == 1,
			Compacted: row.Compacted == 1, ChangeID: row.ChangeID, DeletedAtMs: row.DeletedAtMs.Int64, CreatedAtMs: row.CreatedAtMs,
		})
	}
	return out, nil
}

func (s *SQLStore) SoftDeleteMessages(ctx context.Context, userID, deletedAtMs int64) ([]int64, error) {
	var ids []int64
	if err := s.exec.QueryRowsCtx(ctx, &ids, `SELECT id FROM assistant_message WHERE user_id=? AND deleted_at_ms IS NULL`, userID); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if _, err := s.exec.ExecCtx(ctx, `UPDATE assistant_message SET deleted_at_ms=? WHERE user_id=? AND deleted_at_ms IS NULL`, deletedAtMs, userID); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *SQLStore) MarkMessagesRead(ctx context.Context, userID int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE assistant_message SET unread=0 WHERE user_id=? AND unread=1`, userID)
	return err
}

func (s *SQLStore) MarkMessagesCompacted(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := s.exec.ExecCtx(ctx, `UPDATE assistant_message SET compacted=1 WHERE id IN (`+placeholders(len(ids))+`)`, intsToAny(ids)...)
	return err
}
