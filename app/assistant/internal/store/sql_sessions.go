package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func (s *SQLStore) LockThread(ctx context.Context, userID int64) (*Thread, error) {
	thread, err := s.getThread(ctx, `SELECT user_id, session_id, unread_count, last_message_id, last_message_preview,
		last_message_at_ms, active_run_id, updated_at_ms FROM assistant_thread WHERE user_id = ? FOR UPDATE`, userID)
	if err == nil || err != sqlx.ErrNotFound {
		return thread, err
	}
	now := NowMs()
	if _, err := s.exec.ExecCtx(ctx, `INSERT INTO assistant_thread
		(user_id, session_id, unread_count, last_message_id, last_message_preview, last_message_at_ms, active_run_id, updated_at_ms)
		VALUES (?, 0, 0, 0, '', 0, 0, ?)`, userID, now); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, err
		}
	}
	return s.getThread(ctx, `SELECT user_id, session_id, unread_count, last_message_id, last_message_preview,
		last_message_at_ms, active_run_id, updated_at_ms FROM assistant_thread WHERE user_id = ? FOR UPDATE`, userID)
}

func (s *SQLStore) GetThread(ctx context.Context, userID int64) (*Thread, error) {
	thread, err := s.getThread(ctx, `SELECT user_id, session_id, unread_count, last_message_id, last_message_preview,
		last_message_at_ms, active_run_id, updated_at_ms FROM assistant_thread WHERE user_id = ?`, userID)
	if err == sqlx.ErrNotFound {
		return &Thread{UserID: userID}, nil
	}
	return thread, err
}

func (s *SQLStore) getThread(ctx context.Context, query string, args ...any) (*Thread, error) {
	var row struct {
		UserID             int64  `db:"user_id"`
		SessionID          int64  `db:"session_id"`
		UnreadCount        int64  `db:"unread_count"`
		LastMessageID      int64  `db:"last_message_id"`
		LastMessagePreview string `db:"last_message_preview"`
		LastMessageAtMs    int64  `db:"last_message_at_ms"`
		ActiveRunID        int64  `db:"active_run_id"`
		UpdatedAtMs        int64  `db:"updated_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, query, args...); err != nil {
		return nil, err
	}
	return &Thread{
		UserID: row.UserID, SessionID: row.SessionID, UnreadCount: int32(row.UnreadCount),
		LastMessageID: row.LastMessageID, LastMessagePreview: row.LastMessagePreview,
		LastMessageAtMs: row.LastMessageAtMs, ActiveRunID: row.ActiveRunID, UpdatedAtMs: row.UpdatedAtMs,
	}, nil
}

func (s *SQLStore) SaveThread(ctx context.Context, thread Thread) error {
	_, err := s.exec.ExecCtx(ctx, `INSERT INTO assistant_thread
		(user_id, session_id, unread_count, last_message_id, last_message_preview, last_message_at_ms, active_run_id, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE session_id=VALUES(session_id), unread_count=VALUES(unread_count),
		last_message_id=VALUES(last_message_id), last_message_preview=VALUES(last_message_preview),
		last_message_at_ms=VALUES(last_message_at_ms), active_run_id=VALUES(active_run_id), updated_at_ms=VALUES(updated_at_ms)`,
		thread.UserID, thread.SessionID, thread.UnreadCount, thread.LastMessageID, thread.LastMessagePreview,
		thread.LastMessageAtMs, thread.ActiveRunID, thread.UpdatedAtMs)
	return err
}

func (s *SQLStore) CreateSession(ctx context.Context, session Session) (Session, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO assistant_session
		(user_id, prompt_epoch, prompt_snapshot, tool_snapshot, compact_summary, status, successful_user_turns, created_at_ms, closed_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.UserID, session.PromptEpoch, nullBytes(session.PromptSnapshot), nullBytes(session.ToolSnapshot),
		nullString(session.CompactSummary), session.Status, session.SuccessfulUserTurns, session.CreatedAtMs, nullInt(session.ClosedAtMs))
	if err != nil {
		return Session{}, err
	}
	id, _ := res.LastInsertId()
	session.ID = id
	return session, nil
}

func (s *SQLStore) GetSession(ctx context.Context, id int64) (*Session, error) {
	var row struct {
		ID                  int64          `db:"id"`
		UserID              int64          `db:"user_id"`
		PromptEpoch         int64          `db:"prompt_epoch"`
		PromptSnapshot      []byte         `db:"prompt_snapshot"`
		ToolSnapshot        []byte         `db:"tool_snapshot"`
		CompactSummary      sql.NullString `db:"compact_summary"`
		Status              string         `db:"status"`
		SuccessfulUserTurns int64          `db:"successful_user_turns"`
		CreatedAtMs         int64          `db:"created_at_ms"`
		ClosedAtMs          sql.NullInt64  `db:"closed_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, prompt_epoch, prompt_snapshot, tool_snapshot, compact_summary,
		status, successful_user_turns, created_at_ms, closed_at_ms FROM assistant_session WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return &Session{
		ID: row.ID, UserID: row.UserID, PromptEpoch: int(row.PromptEpoch), PromptSnapshot: row.PromptSnapshot,
		ToolSnapshot: row.ToolSnapshot, CompactSummary: row.CompactSummary.String, Status: row.Status,
		SuccessfulUserTurns: int(row.SuccessfulUserTurns), CreatedAtMs: row.CreatedAtMs, ClosedAtMs: row.ClosedAtMs.Int64,
	}, nil
}

func (s *SQLStore) UpdateSession(ctx context.Context, session Session) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE assistant_session SET prompt_epoch=?, prompt_snapshot=?, tool_snapshot=?,
		compact_summary=?, status=?, successful_user_turns=?, closed_at_ms=? WHERE id=?`,
		session.PromptEpoch, nullBytes(session.PromptSnapshot), nullBytes(session.ToolSnapshot),
		nullString(session.CompactSummary), session.Status, session.SuccessfulUserTurns, nullInt(session.ClosedAtMs), session.ID)
	return err
}

func (s *SQLStore) CloseSession(ctx context.Context, id int64, closedAtMs int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE assistant_session SET status=?, closed_at_ms=? WHERE id=?`, SessionClosed, closedAtMs, id)
	return err
}
