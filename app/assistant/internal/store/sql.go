package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type execer interface {
	ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
	QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error
}

type SQLStore struct {
	exec execer
	conn sqlx.SqlConn
}

func NewSQLStore(conn sqlx.SqlConn) *SQLStore {
	return &SQLStore{exec: conn, conn: conn}
}

func (s *SQLStore) Transact(ctx context.Context, fn func(ctx context.Context, tx Store) error) error {
	if s.conn == nil {
		return fn(ctx, s)
	}
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		return fn(ctx, &SQLStore{exec: session})
	})
}

func (s *SQLStore) RunStep(ctx context.Context, fence LeaseFence, fn func(ctx context.Context, tx Store) error) error {
	if fence.RunID <= 0 || strings.TrimSpace(fence.Owner) == "" || fence.Generation <= 0 {
		return ErrLeaseLost
	}
	return s.Transact(ctx, func(ctx context.Context, tx Store) error {
		sqlTx, ok := tx.(*SQLStore)
		if !ok {
			return fmt.Errorf("run step requires SQL store")
		}
		var row struct {
			ID int64 `db:"id"`
		}
		err := sqlTx.exec.QueryRowCtx(ctx, &row, `SELECT id FROM agent_run
			WHERE id=? AND lease_owner=? AND lease_generation=? AND status='running' AND lease_until_ms>=?
			FOR UPDATE`, fence.RunID, fence.Owner, fence.Generation, NowMs())
		if err == sqlx.ErrNotFound {
			return ErrLeaseLost
		}
		if err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

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

func (s *SQLStore) InsertRun(ctx context.Context, run Run) (Run, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_run
		(user_id, session_id, request_id, source, status, phase, priority, queued_payload, lease_owner, lease_generation, lease_until_ms,
		 heartbeat_at_ms, cancel_requested, consent_version, input_version, prompt_epoch, model, rounds, tool_calls, input_tokens,
			 output_tokens, cache_tokens, cache_write_tokens, reasoning_tokens, last_prompt_tokens, usage_estimated, cost_usd, started_at_ms, ended_at_ms, last_activity_at_ms, error_code, created_at_ms, client_protocol_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.UserID, run.SessionID, run.RequestID, run.Source, run.Status, run.Phase, run.Priority, nullBytes(run.QueuedPayload),
		nullString(run.LeaseOwner), run.LeaseGeneration, nullInt(run.LeaseUntilMs), nullInt(run.HeartbeatAtMs), boolToInt(run.CancelRequested),
		run.ConsentVersion, run.InputVersion, run.PromptEpoch, nullString(run.Model), run.Rounds, run.ToolCalls, run.InputTokens, run.OutputTokens, run.CacheTokens,
		run.CacheWriteTokens, run.ReasoningTokens, run.LastPromptTokens, boolToInt(run.UsageEstimated),
		run.CostUSD, nullInt(run.StartedAtMs), nullInt(run.EndedAtMs), nullInt(run.LastActivityAtMs), nullString(run.ErrorCode), run.CreatedAtMs, run.ClientProtocolVersion)
	if err != nil {
		return Run{}, err
	}
	id, _ := res.LastInsertId()
	run.ID = id
	return run, nil
}

func (s *SQLStore) GetRun(ctx context.Context, id int64) (*Run, error) {
	return s.scanRun(ctx, `SELECT * FROM (`+runSelect+`) r WHERE r.id=?`, id)
}

func (s *SQLStore) GetRunByRequestID(ctx context.Context, userID int64, requestID string) (*Run, error) {
	run, err := s.scanRun(ctx, `SELECT * FROM (`+runSelect+`) r WHERE r.user_id=? AND r.request_id=?`, userID, requestID)
	if err == sqlx.ErrNotFound {
		return nil, nil
	}
	return run, err
}

const runSelect = `SELECT id, user_id, session_id, request_id, source, status, phase, priority, queued_payload, lease_owner,
	lease_generation, lease_until_ms, heartbeat_at_ms, cancel_requested, consent_version, input_version, prompt_epoch, model, rounds, tool_calls, input_tokens, output_tokens,
	cache_tokens, cache_write_tokens, reasoning_tokens, last_prompt_tokens, usage_estimated, cost_usd, started_at_ms, ended_at_ms, last_activity_at_ms, error_code, created_at_ms, client_protocol_version FROM agent_run`

func (s *SQLStore) scanRun(ctx context.Context, query string, args ...any) (*Run, error) {
	var row runRow
	if err := s.exec.QueryRowCtx(ctx, &row, query, args...); err != nil {
		return nil, err
	}
	out := row.toRun()
	return &out, nil
}

type runRow struct {
	ClientProtocolVersion int            `db:"client_protocol_version"`
	ID                    int64          `db:"id"`
	UserID                int64          `db:"user_id"`
	SessionID             int64          `db:"session_id"`
	RequestID             string         `db:"request_id"`
	Source                string         `db:"source"`
	Status                string         `db:"status"`
	Phase                 string         `db:"phase"`
	Priority              int64          `db:"priority"`
	QueuedPayload         []byte         `db:"queued_payload"`
	LeaseOwner            sql.NullString `db:"lease_owner"`
	LeaseGeneration       int64          `db:"lease_generation"`
	LeaseUntilMs          sql.NullInt64  `db:"lease_until_ms"`
	HeartbeatAtMs         sql.NullInt64  `db:"heartbeat_at_ms"`
	CancelRequested       int64          `db:"cancel_requested"`
	ConsentVersion        int32          `db:"consent_version"`
	InputVersion          int64          `db:"input_version"`
	PromptEpoch           int64          `db:"prompt_epoch"`
	Model                 sql.NullString `db:"model"`
	Rounds                int64          `db:"rounds"`
	ToolCalls             int64          `db:"tool_calls"`
	InputTokens           int64          `db:"input_tokens"`
	OutputTokens          int64          `db:"output_tokens"`
	CacheTokens           int64          `db:"cache_tokens"`
	CacheWriteTokens      int64          `db:"cache_write_tokens"`
	ReasoningTokens       int64          `db:"reasoning_tokens"`
	LastPromptTokens      int64          `db:"last_prompt_tokens"`
	UsageEstimated        int            `db:"usage_estimated"`
	CostUSD               float64        `db:"cost_usd"`
	StartedAtMs           sql.NullInt64  `db:"started_at_ms"`
	EndedAtMs             sql.NullInt64  `db:"ended_at_ms"`
	LastActivityAtMs      sql.NullInt64  `db:"last_activity_at_ms"`
	ErrorCode             sql.NullString `db:"error_code"`
	CreatedAtMs           int64          `db:"created_at_ms"`
}

func (row runRow) toRun() Run {
	return Run{
		ClientProtocolVersion: row.ClientProtocolVersion,
		ID:                    row.ID, UserID: row.UserID, SessionID: row.SessionID, RequestID: row.RequestID, Source: row.Source,
		Status: row.Status, Phase: row.Phase, Priority: int(row.Priority), QueuedPayload: row.QueuedPayload,
		LeaseOwner: row.LeaseOwner.String, LeaseGeneration: row.LeaseGeneration, LeaseUntilMs: row.LeaseUntilMs.Int64, HeartbeatAtMs: row.HeartbeatAtMs.Int64,
		CancelRequested: row.CancelRequested == 1, ConsentVersion: row.ConsentVersion, InputVersion: row.InputVersion,
		PromptEpoch: int(row.PromptEpoch), Model: row.Model.String,
		Rounds: int(row.Rounds), ToolCalls: int(row.ToolCalls), InputTokens: row.InputTokens, OutputTokens: row.OutputTokens,
		CacheTokens: row.CacheTokens, CacheWriteTokens: row.CacheWriteTokens, ReasoningTokens: row.ReasoningTokens,
		LastPromptTokens: row.LastPromptTokens, UsageEstimated: row.UsageEstimated == 1, CostUSD: row.CostUSD, StartedAtMs: row.StartedAtMs.Int64, EndedAtMs: row.EndedAtMs.Int64,
		LastActivityAtMs: row.LastActivityAtMs.Int64, ErrorCode: row.ErrorCode.String, CreatedAtMs: row.CreatedAtMs,
	}
}

func (s *SQLStore) UpdateRun(ctx context.Context, run Run) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET status=?, phase=?,
		cancel_requested=cancel_requested OR ?, prompt_epoch=?, model=?, rounds=?, tool_calls=?, input_tokens=?, output_tokens=?,
		cache_tokens=?, cache_write_tokens=?, reasoning_tokens=?, last_prompt_tokens=?, usage_estimated=?, cost_usd=?, started_at_ms=?, ended_at_ms=?, last_activity_at_ms=?, error_code=? WHERE id=?`,
		run.Status, run.Phase, boolToInt(run.CancelRequested), run.PromptEpoch, nullString(run.Model), run.Rounds,
		run.ToolCalls, run.InputTokens, run.OutputTokens, run.CacheTokens, run.CacheWriteTokens, run.ReasoningTokens,
		run.LastPromptTokens, boolToInt(run.UsageEstimated), run.CostUSD, nullInt(run.StartedAtMs),
		nullInt(run.EndedAtMs), nullInt(run.LastActivityAtMs), nullString(run.ErrorCode), run.ID)
	return err
}

func (s *SQLStore) SetRunInput(ctx context.Context, runID int64, payload []byte, lastActivityMs int64) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE agent_run
		SET queued_payload=?, input_version=input_version+1, last_activity_at_ms=?
		WHERE id=? AND status IN ('running','queued')`, nullBytes(payload), lastActivityMs, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sqlx.ErrNotFound
	}
	return nil
}

func (s *SQLStore) RequestCancel(ctx context.Context, userID, runID int64) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET cancel_requested=1 WHERE id=? AND user_id=?`, runID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sqlx.ErrNotFound
	}
	return nil
}

func (s *SQLStore) RequestCancelAll(ctx context.Context, userID int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET cancel_requested=1
		WHERE user_id=? AND status IN ('queued','running','waiting_input')`, userID)
	return err
}

func (s *SQLStore) CancelOpenBackground(ctx context.Context, userID int64, sources []string) ([]Run, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	// Accept may redirect the active foreground run after it locks the thread.
	// Lock every open run first so worker completion and input acceptance both
	// use agent_run -> assistant_thread.
	query := runSelect + ` WHERE user_id=? AND status IN ('queued','running','waiting_input') ORDER BY id FOR UPDATE`
	var rows []runRow
	if err := s.exec.QueryRowsCtx(ctx, &rows, query, userID); err != nil {
		return nil, err
	}
	wanted := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		wanted[source] = struct{}{}
	}
	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		run := row.toRun()
		if _, ok := wanted[run.Source]; !ok {
			continue
		}
		if _, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET cancel_requested=1 WHERE id=?`, run.ID); err != nil {
			return nil, err
		}
		run.CancelRequested = true
		out = append(out, run)
	}
	return out, nil
}

func (s *SQLStore) Claim(ctx context.Context, owner string, nowMs, leaseMs int64) (*Run, error) {
	var claimed *Run
	err := s.Transact(ctx, func(ctx context.Context, tx Store) error {
		sqlTx, ok := tx.(*SQLStore)
		if !ok {
			return fmt.Errorf("claim requires SQL store")
		}
		var row runRow
		err := sqlTx.exec.QueryRowCtx(ctx, &row, runSelect+`
			WHERE (status='queued' OR (status='running' AND (lease_until_ms IS NULL OR lease_until_ms < ?)))
			ORDER BY priority ASC, created_at_ms ASC LIMIT 1 FOR UPDATE SKIP LOCKED`, nowMs)
		if err != nil {
			return err
		}
		run := row.toRun()
		run.Status = StatusRunning
		if run.StartedAtMs == 0 {
			run.StartedAtMs = nowMs
		}
		run.LeaseOwner = owner
		run.LeaseGeneration++
		run.LeaseUntilMs = nowMs + leaseMs
		run.HeartbeatAtMs = nowMs
		run.LastActivityAtMs = nowMs
		if _, err := sqlTx.exec.ExecCtx(ctx, `UPDATE agent_run SET status=?, lease_owner=?, lease_generation=?,
			lease_until_ms=?, heartbeat_at_ms=?, started_at_ms=?, last_activity_at_ms=? WHERE id=?`,
			run.Status, run.LeaseOwner, run.LeaseGeneration, run.LeaseUntilMs, run.HeartbeatAtMs,
			run.StartedAtMs, run.LastActivityAtMs, run.ID); err != nil {
			return err
		}
		claimed = &run
		return nil
	})
	if err == sqlx.ErrNotFound {
		return nil, nil
	}
	return claimed, err
}

func (s *SQLStore) RenewLease(ctx context.Context, runID int64, owner string, generation, leaseUntilMs, heartbeatMs int64) (bool, error) {
	res, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET lease_until_ms=?, heartbeat_at_ms=?
		WHERE id=? AND lease_owner=? AND lease_generation=? AND status='running' AND lease_until_ms>=?`,
		leaseUntilMs, heartbeatMs, runID, owner, generation, heartbeatMs)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *SQLStore) AgentConsent(ctx context.Context, userID int64) (int32, bool, error) {
	var row struct {
		Granted        int64 `db:"granted"`
		ConsentVersion int32 `db:"consent_version"`
	}
	err := s.exec.QueryRowCtx(ctx, &row, `SELECT granted, consent_version
		FROM xbh_user.agent_capability_consent WHERE user_id=? LIMIT 1 FOR SHARE`, userID)
	if err == sqlx.ErrNotFound {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.ConsentVersion, row.Granted == 1 && row.ConsentVersion > 0, nil
}

func (s *SQLStore) OldestQueuedAgeMs(ctx context.Context, nowMs int64) (int64, error) {
	var row struct {
		Created sql.NullInt64 `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT MIN(created_at_ms) AS created_at_ms FROM agent_run WHERE status='queued'`); err != nil {
		if err == sqlx.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	if !row.Created.Valid {
		return 0, nil
	}
	age := nowMs - row.Created.Int64
	if age < 0 {
		return 0, nil
	}
	return age, nil
}

func (s *SQLStore) InsertEvent(ctx context.Context, runID int64, eventType string, payload []byte, createdAtMs int64) (Event, error) {
	var seqRow struct {
		Seq int64 `db:"seq"`
	}
	if err := s.exec.QueryRowCtx(ctx, &seqRow, `SELECT IFNULL(MAX(seq),0)+1 AS seq FROM agent_run_event WHERE run_id=?`, runID); err != nil {
		return Event{}, err
	}
	seq := seqRow.Seq
	terminalRunID := int64(0)
	if eventType == EventDone || eventType == EventError {
		terminalRunID = runID
	}
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_run_event
		(run_id, seq, type, terminal_run_id, payload_json, created_at_ms) VALUES (?, ?, ?, ?, ?, ?)`,
		runID, seq, eventType, nullInt(terminalRunID), nullBytes(payload), createdAtMs)
	if err != nil {
		return Event{}, err
	}
	id, _ := res.LastInsertId()
	return Event{ID: id, RunID: runID, Seq: seq, Type: eventType, PayloadJSON: payload, CreatedAtMs: createdAtMs}, nil
}

func (s *SQLStore) ListEventsAfter(ctx context.Context, runID, afterSeq int64) ([]Event, error) {
	var rows []struct {
		ID          int64  `db:"id"`
		RunID       int64  `db:"run_id"`
		Seq         int64  `db:"seq"`
		Type        string `db:"type"`
		PayloadJSON []byte `db:"payload_json"`
		CreatedAtMs int64  `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, run_id, seq, type, payload_json, created_at_ms FROM agent_run_event
		WHERE run_id=? AND seq>? ORDER BY seq ASC`, runID, afterSeq); err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, Event{ID: row.ID, RunID: row.RunID, Seq: row.Seq, Type: row.Type, PayloadJSON: row.PayloadJSON, CreatedAtMs: row.CreatedAtMs})
	}
	return out, nil
}

func (s *SQLStore) MaxEventSeq(ctx context.Context, runID int64) (int64, error) {
	var row struct {
		Seq int64 `db:"seq"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT IFNULL(MAX(seq),0) AS seq FROM agent_run_event WHERE run_id=?`, runID); err != nil {
		if err == sqlx.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return row.Seq, nil
}

func (s *SQLStore) InsertToolCall(ctx context.Context, call ToolCall) (ToolCall, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_tool_call (run_id, call_id, tool, args_json, canonical_args_digest, status, result_json, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		call.RunID, call.CallID, call.Tool, nullString(call.ArgsJSON), call.CanonicalArgsDigest, call.Status, nullString(call.ResultJSON), call.CreatedAtMs)
	if err != nil {
		return ToolCall{}, err
	}
	id, _ := res.LastInsertId()
	call.ID = id
	return call, nil
}

func (s *SQLStore) GetToolCall(ctx context.Context, runID int64, callID string) (*ToolCall, error) {
	var row struct {
		ID                  int64          `db:"id"`
		RunID               int64          `db:"run_id"`
		CallID              string         `db:"call_id"`
		Tool                string         `db:"tool"`
		ArgsJSON            sql.NullString `db:"args_json"`
		CanonicalArgsDigest string         `db:"canonical_args_digest"`
		Status              string         `db:"status"`
		ResultJSON          sql.NullString `db:"result_json"`
		CreatedAtMs         int64          `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, run_id, call_id, tool, args_json, canonical_args_digest, status, result_json, created_at_ms
		FROM agent_tool_call WHERE run_id=? AND call_id=?`, runID, callID); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &ToolCall{
		ID: row.ID, RunID: row.RunID, CallID: row.CallID, Tool: row.Tool, ArgsJSON: row.ArgsJSON.String,
		CanonicalArgsDigest: row.CanonicalArgsDigest, Status: row.Status, ResultJSON: row.ResultJSON.String, CreatedAtMs: row.CreatedAtMs,
	}, nil
}

func (s *SQLStore) UpdateToolCall(ctx context.Context, call ToolCall) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_tool_call SET status=?, result_json=? WHERE run_id=? AND call_id=?`,
		call.Status, nullString(call.ResultJSON), call.RunID, call.CallID)
	return err
}

func (s *SQLStore) ListToolCalls(ctx context.Context, runID int64) ([]ToolCall, error) {
	var rows []struct {
		ID                  int64          `db:"id"`
		RunID               int64          `db:"run_id"`
		CallID              string         `db:"call_id"`
		Tool                string         `db:"tool"`
		ArgsJSON            sql.NullString `db:"args_json"`
		CanonicalArgsDigest string         `db:"canonical_args_digest"`
		Status              string         `db:"status"`
		ResultJSON          sql.NullString `db:"result_json"`
		CreatedAtMs         int64          `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, run_id, call_id, tool, args_json, canonical_args_digest, status, result_json, created_at_ms
		FROM agent_tool_call WHERE run_id=? ORDER BY id ASC`, runID); err != nil {
		return nil, err
	}
	out := make([]ToolCall, 0, len(rows))
	for _, row := range rows {
		out = append(out, ToolCall{
			ID: row.ID, RunID: row.RunID, CallID: row.CallID, Tool: row.Tool, ArgsJSON: row.ArgsJSON.String,
			CanonicalArgsDigest: row.CanonicalArgsDigest, Status: row.Status, ResultJSON: row.ResultJSON.String, CreatedAtMs: row.CreatedAtMs,
		})
	}
	return out, nil
}

func (s *SQLStore) GetJournal(ctx context.Context, userID int64, requestID, tool, digest string) (*Journal, error) {
	var row struct {
		ID                  int64          `db:"id"`
		UserID              int64          `db:"user_id"`
		RequestID           string         `db:"request_id"`
		Tool                string         `db:"tool"`
		CanonicalArgsDigest string         `db:"canonical_args_digest"`
		RunID               int64          `db:"run_id"`
		LeaseGeneration     int64          `db:"lease_generation"`
		ResultJSON          sql.NullString `db:"result_json"`
		Status              string         `db:"status"`
		CreatedAtMs         int64          `db:"created_at_ms"`
		UpdatedAtMs         int64          `db:"updated_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, request_id, tool, canonical_args_digest, run_id,
		lease_generation, result_json, status, created_at_ms, updated_at_ms
		FROM agent_command_journal WHERE user_id=? AND request_id=? AND tool=? AND canonical_args_digest=?`,
		userID, requestID, tool, digest); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &Journal{
		ID: row.ID, UserID: row.UserID, RequestID: row.RequestID, Tool: row.Tool, CanonicalArgsDigest: row.CanonicalArgsDigest,
		RunID: row.RunID, LeaseGeneration: row.LeaseGeneration, ResultJSON: row.ResultJSON.String,
		Status: row.Status, CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs,
	}, nil
}

func (s *SQLStore) ReserveJournal(ctx context.Context, row Journal) (*Journal, bool, error) {
	existing, err := s.GetJournal(ctx, row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.Status != JournalSuccess && existing.LeaseGeneration < row.LeaseGeneration {
			res, updateErr := s.exec.ExecCtx(ctx, `UPDATE agent_command_journal
				SET run_id=?, lease_generation=?, status=?, result_json=NULL, updated_at_ms=?
				WHERE id=? AND status<>? AND lease_generation<?`, row.RunID, row.LeaseGeneration,
				JournalPending, row.UpdatedAtMs, existing.ID, JournalSuccess, row.LeaseGeneration)
			if updateErr != nil {
				return nil, false, updateErr
			}
			n, rowsErr := res.RowsAffected()
			if rowsErr != nil {
				return nil, false, rowsErr
			}
			if n == 1 {
				row.ID = existing.ID
				row.CreatedAtMs = existing.CreatedAtMs
				row.Status = JournalPending
				row.ResultJSON = ""
				row.Takeover = true
				return &row, true, nil
			}
		}
		return existing, false, nil
	}
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_command_journal
		(user_id, request_id, tool, canonical_args_digest, run_id, lease_generation, result_json, status, created_at_ms, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest, row.RunID, row.LeaseGeneration,
		nullString(row.ResultJSON), row.Status, row.CreatedAtMs, row.UpdatedAtMs)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			existing, err = s.GetJournal(ctx, row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest)
			return existing, false, err
		}
		return nil, false, err
	}
	id, _ := res.LastInsertId()
	row.ID = id
	return &row, true, nil
}

func (s *SQLStore) CompleteJournal(ctx context.Context, id int64, status, resultJSON string) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_command_journal SET status=?, result_json=?, updated_at_ms=? WHERE id=?`,
		status, resultJSON, NowMs(), id)
	return err
}

func (s *SQLStore) ListSuccessfulJournal(ctx context.Context, userID int64, requestID string) ([]Journal, error) {
	var rows []struct {
		ID                  int64          `db:"id"`
		UserID              int64          `db:"user_id"`
		RequestID           string         `db:"request_id"`
		Tool                string         `db:"tool"`
		CanonicalArgsDigest string         `db:"canonical_args_digest"`
		RunID               int64          `db:"run_id"`
		LeaseGeneration     int64          `db:"lease_generation"`
		ResultJSON          sql.NullString `db:"result_json"`
		Status              string         `db:"status"`
		CreatedAtMs         int64          `db:"created_at_ms"`
		UpdatedAtMs         int64          `db:"updated_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, request_id, tool, canonical_args_digest, run_id,
		lease_generation, result_json, status, created_at_ms, updated_at_ms
		FROM agent_command_journal WHERE user_id=? AND request_id=? AND status=?`, userID, requestID, JournalSuccess); err != nil {
		return nil, err
	}
	out := make([]Journal, 0, len(rows))
	for _, row := range rows {
		out = append(out, Journal{
			ID: row.ID, UserID: row.UserID, RequestID: row.RequestID, Tool: row.Tool, CanonicalArgsDigest: row.CanonicalArgsDigest,
			RunID: row.RunID, LeaseGeneration: row.LeaseGeneration, ResultJSON: row.ResultJSON.String,
			Status: row.Status, CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs,
		})
	}
	return out, nil
}

func (s *SQLStore) InsertSource(ctx context.Context, src Source) (Source, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_source_ledger (run_id, handle, kind, authority_id, revision, payload_json, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		src.RunID, src.Handle, src.Kind, src.AuthorityID, src.Revision, nullString(src.PayloadJSON), src.CreatedAtMs)
	if err != nil {
		return Source{}, err
	}
	id, _ := res.LastInsertId()
	src.ID = id
	return src, nil
}

func (s *SQLStore) GetSources(ctx context.Context, runID int64, handles []string) ([]Source, error) {
	if len(handles) == 0 {
		return nil, nil
	}
	args := append([]any{runID}, stringsToAny(handles)...)
	return s.scanSources(ctx, `SELECT id, run_id, handle, kind, authority_id, revision, payload_json, created_at_ms
		FROM agent_source_ledger WHERE run_id=? AND handle IN (`+placeholders(len(handles))+`)`, args...)
}

func (s *SQLStore) ListSources(ctx context.Context, runID int64) ([]Source, error) {
	return s.scanSources(ctx, `SELECT id, run_id, handle, kind, authority_id, revision, payload_json, created_at_ms
		FROM agent_source_ledger WHERE run_id=? ORDER BY id ASC`, runID)
}

func (s *SQLStore) scanSources(ctx context.Context, query string, args ...any) ([]Source, error) {
	var rows []struct {
		ID          int64          `db:"id"`
		RunID       int64          `db:"run_id"`
		Handle      string         `db:"handle"`
		Kind        string         `db:"kind"`
		AuthorityID string         `db:"authority_id"`
		Revision    int64          `db:"revision"`
		PayloadJSON sql.NullString `db:"payload_json"`
		CreatedAtMs int64          `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]Source, 0, len(rows))
	for _, row := range rows {
		out = append(out, Source{
			ID: row.ID, RunID: row.RunID, Handle: row.Handle, Kind: row.Kind, AuthorityID: row.AuthorityID,
			Revision: row.Revision, PayloadJSON: row.PayloadJSON.String, CreatedAtMs: row.CreatedAtMs,
		})
	}
	return out, nil
}

func (s *SQLStore) InsertConfirmation(ctx context.Context, row Confirmation) (Confirmation, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_confirmation
		(user_id, session_id, run_id, call_id, tool, canonical_args_digest, target_revision, status, created_at_ms, resolved_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.UserID, row.SessionID, row.RunID, row.CallID, row.Tool, row.CanonicalArgsDigest, row.TargetRevision,
		row.Status, row.CreatedAtMs, nullInt(row.ResolvedAtMs))
	if err != nil {
		return Confirmation{}, err
	}
	id, _ := res.LastInsertId()
	row.ID = id
	return row, nil
}

func (s *SQLStore) GetConfirmation(ctx context.Context, runID int64, callID string) (*Confirmation, error) {
	var row struct {
		ID                  int64         `db:"id"`
		UserID              int64         `db:"user_id"`
		SessionID           int64         `db:"session_id"`
		RunID               int64         `db:"run_id"`
		CallID              string        `db:"call_id"`
		Tool                string        `db:"tool"`
		CanonicalArgsDigest string        `db:"canonical_args_digest"`
		TargetRevision      int64         `db:"target_revision"`
		Status              string        `db:"status"`
		CreatedAtMs         int64         `db:"created_at_ms"`
		ResolvedAtMs        sql.NullInt64 `db:"resolved_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, session_id, run_id, call_id, tool, canonical_args_digest, target_revision, status, created_at_ms, resolved_at_ms
		FROM agent_confirmation WHERE run_id=? AND call_id=?`, runID, callID); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &Confirmation{
		ID: row.ID, UserID: row.UserID, SessionID: row.SessionID, RunID: row.RunID, CallID: row.CallID, Tool: row.Tool,
		CanonicalArgsDigest: row.CanonicalArgsDigest, TargetRevision: row.TargetRevision, Status: row.Status,
		CreatedAtMs: row.CreatedAtMs, ResolvedAtMs: row.ResolvedAtMs.Int64,
	}, nil
}

func (s *SQLStore) ResolveConfirmation(ctx context.Context, userID, runID int64, callID, digest string, approved bool, nowMs int64) (*Confirmation, error) {
	status := ConfirmRejected
	if approved {
		status = ConfirmApproved
	}
	res, err := s.exec.ExecCtx(ctx, `UPDATE agent_confirmation SET status=?, resolved_at_ms=?
		WHERE user_id=? AND run_id=? AND call_id=? AND canonical_args_digest=? AND status=?`,
		status, nowMs, userID, runID, callID, digest, ConfirmPending)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return s.GetConfirmation(ctx, runID, callID)
	}
	return s.GetConfirmation(ctx, runID, callID)
}

func (s *SQLStore) GetInputCommand(ctx context.Context, userID int64, requestID string) (*InputCommand, error) {
	var row InputCommand
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, request_id, session_id, message_id, run_id, disposition, created_at_ms
		FROM assistant_input_command WHERE user_id=? AND request_id=?`, userID, requestID); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (s *SQLStore) InsertInputCommand(ctx context.Context, command InputCommand) (InputCommand, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO assistant_input_command
		(user_id, request_id, session_id, message_id, run_id, disposition, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, command.UserID, command.RequestID, command.SessionID,
		command.MessageID, command.RunID, command.Disposition, command.CreatedAtMs)
	if err != nil {
		return InputCommand{}, err
	}
	command.ID, _ = res.LastInsertId()
	return command, nil
}

func (s *SQLStore) CountQueue(ctx context.Context, runID int64) (int, error) {
	var row struct {
		N int64 `db:"n"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT COUNT(*) AS n FROM agent_input_queue WHERE run_id=?`, runID); err != nil {
		return 0, err
	}
	return int(row.N), nil
}

func (s *SQLStore) Enqueue(ctx context.Context, item QueueItem) (QueueItem, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_input_queue (user_id, run_id, message_id, created_at_ms) VALUES (?, ?, ?, ?)`,
		item.UserID, item.RunID, item.MessageID, item.CreatedAtMs)
	if err != nil {
		return QueueItem{}, err
	}
	id, _ := res.LastInsertId()
	item.ID = id
	return item, nil
}

func (s *SQLStore) ListQueue(ctx context.Context, runID int64) ([]QueueItem, error) {
	var rows []struct {
		ID          int64 `db:"id"`
		UserID      int64 `db:"user_id"`
		RunID       int64 `db:"run_id"`
		MessageID   int64 `db:"message_id"`
		CreatedAtMs int64 `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, run_id, message_id, created_at_ms FROM agent_input_queue WHERE run_id=? ORDER BY id ASC`, runID); err != nil {
		return nil, err
	}
	out := make([]QueueItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, QueueItem{ID: row.ID, UserID: row.UserID, RunID: row.RunID, MessageID: row.MessageID, CreatedAtMs: row.CreatedAtMs})
	}
	return out, nil
}

func (s *SQLStore) DeleteQueueThrough(ctx context.Context, runID, maxID int64) error {
	if maxID <= 0 {
		return nil
	}
	_, err := s.exec.ExecCtx(ctx, `DELETE FROM agent_input_queue WHERE run_id=? AND id<=?`, runID, maxID)
	return err
}

func (s *SQLStore) DeleteQueue(ctx context.Context, runID int64) error {
	_, err := s.exec.ExecCtx(ctx, `DELETE FROM agent_input_queue WHERE run_id=?`, runID)
	return err
}

func (s *SQLStore) InsertAlert(ctx context.Context, alert Alert) (bool, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT IGNORE INTO agent_run_alert (run_id, level, dimension, created_at_ms) VALUES (?, ?, ?, ?)`,
		alert.RunID, alert.Level, alert.Dimension, alert.CreatedAtMs)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

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

func (s *SQLStore) UpsertDeliveryBucket(ctx context.Context, userID, hitID, windowStartMs, nowMs int64) (DeliveryBucket, error) {
	var existing DeliveryBucket
	err := s.exec.QueryRowCtx(ctx, &struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}{}, `SELECT id FROM watch_delivery_bucket WHERE user_id=? AND window_start_ms=?`, userID, windowStartMs)
	_ = err
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_delivery_bucket (user_id, window_start_ms, status, hit_ids, run_id, created_at_ms)
		VALUES (?, ?, 'pending', ?, 0, ?)
		ON DUPLICATE KEY UPDATE hit_ids = JSON_ARRAY_APPEND(IFNULL(hit_ids, JSON_ARRAY()), '$', ?)`,
		userID, windowStartMs, mustJSON([]int64{hitID}), nowMs, hitID)
	if err != nil {
		return DeliveryBucket{}, err
	}
	id, _ := res.LastInsertId()
	if id == 0 {
		var row struct {
			ID            int64  `db:"id"`
			UserID        int64  `db:"user_id"`
			WindowStartMs int64  `db:"window_start_ms"`
			NotBeforeMs   int64  `db:"not_before_ms"`
			Status        string `db:"status"`
			HitIDs        []byte `db:"hit_ids"`
			RunID         int64  `db:"run_id"`
			CreatedAtMs   int64  `db:"created_at_ms"`
		}
		if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
			FROM watch_delivery_bucket WHERE user_id=? AND window_start_ms=?`, userID, windowStartMs); err != nil {
			return DeliveryBucket{}, err
		}
		existing = DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, NotBeforeMs: row.NotBeforeMs, Status: row.Status,
			HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs}
		return existing, nil
	}
	return DeliveryBucket{ID: id, UserID: userID, WindowStartMs: windowStartMs, Status: "pending", HitIDs: []int64{hitID}, CreatedAtMs: nowMs}, nil
}

func (s *SQLStore) GetBucket(ctx context.Context, id int64) (*DeliveryBucket, error) {
	return s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms FROM watch_delivery_bucket WHERE id=?`, id)
}

func (s *SQLStore) GetPendingBucket(ctx context.Context, userID int64) (*DeliveryBucket, error) {
	return s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE user_id=? AND status IN ('pending','deferred') ORDER BY window_start_ms ASC LIMIT 1`, userID)
}

func (s *SQLStore) ListDueBuckets(ctx context.Context, nowMs, windowMs int64) ([]DeliveryBucket, error) {
	var rows []struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		NotBeforeMs   int64  `db:"not_before_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE status IN ('pending','deferred')
		AND ((not_before_ms=0 AND window_start_ms + ? <= ?) OR (not_before_ms>0 AND not_before_ms <= ?))
		ORDER BY window_start_ms ASC LIMIT 50`, windowMs, nowMs, nowMs); err != nil {
		return nil, err
	}
	out := make([]DeliveryBucket, 0, len(rows))
	for _, row := range rows {
		out = append(out, DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, NotBeforeMs: row.NotBeforeMs, Status: row.Status,
			HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs})
	}
	return out, nil
}

func (s *SQLStore) scanBucket(ctx context.Context, query string, args ...any) (*DeliveryBucket, error) {
	var row struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		NotBeforeMs   int64  `db:"not_before_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, query, args...); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, NotBeforeMs: row.NotBeforeMs, Status: row.Status,
		HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs}, nil
}

func (s *SQLStore) MarkBucketScheduled(ctx context.Context, id, runID int64) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='scheduled', run_id=?, not_before_ms=0
		WHERE id=? AND status IN ('pending','deferred') AND run_id=0`, runID, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("watch bucket %d schedule CAS failed", id)
	}
	return nil
}

func (s *SQLStore) MarkBucketSent(ctx context.Context, id, runID int64) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='sent' WHERE id=? AND run_id=? AND status='scheduled'`, id, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("watch bucket %d is not scheduled for run %d", id, runID)
	}
	return nil
}

func (s *SQLStore) DeferBucket(ctx context.Context, id, notBeforeMs int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='deferred', run_id=0, not_before_ms=? WHERE id=? AND status IN ('pending','deferred')`, notBeforeMs, id)
	return err
}

func (s *SQLStore) DismissBucket(ctx context.Context, id, runID int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.DismissBucket(ctx, id, runID) })
	}
	query := `UPDATE watch_delivery_bucket SET status='discarded', run_id=0, not_before_ms=0
		WHERE id=? AND run_id=0 AND status IN ('pending','deferred')`
	args := []any{id}
	if runID > 0 {
		query = `UPDATE watch_delivery_bucket SET status='discarded', run_id=0, not_before_ms=0
			WHERE id=? AND run_id=? AND status='scheduled'`
		args = append(args, runID)
	}
	res, err := s.exec.ExecCtx(ctx, query, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	return s.releaseWatchQuota(ctx, id, false)
}

func (s *SQLStore) ResetBucket(ctx context.Context, id, runID int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.ResetBucket(ctx, id, runID) })
	}
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0, not_before_ms=0 WHERE id=? AND run_id=? AND status='scheduled'`, id, runID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	return s.releaseWatchQuota(ctx, id, false)
}

func (s *SQLStore) RequeueFailedBuckets(ctx context.Context) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.RequeueFailedBuckets(ctx) })
	}
	var candidates []struct {
		BucketID int64 `db:"bucket_id"`
		RunID    int64 `db:"run_id"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &candidates, `SELECT b.id AS bucket_id, b.run_id FROM watch_delivery_bucket b
		JOIN agent_run r ON r.id=b.run_id WHERE b.status='scheduled' AND r.status IN ('error','cancelled')
		ORDER BY b.run_id, b.id`); err != nil {
		return err
	}
	for _, candidate := range candidates {
		var run struct {
			Status string `db:"status"`
		}
		if err := s.exec.QueryRowCtx(ctx, &run, `SELECT status FROM agent_run WHERE id=? FOR UPDATE`, candidate.RunID); err != nil {
			if err == sqlx.ErrNotFound {
				continue
			}
			return err
		}
		if run.Status != StatusError && run.Status != StatusCancelled {
			continue
		}
		bucket, err := s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
			FROM watch_delivery_bucket WHERE id=? AND run_id=? FOR UPDATE`, candidate.BucketID, candidate.RunID)
		if err != nil {
			return err
		}
		if bucket == nil || bucket.Status != "scheduled" {
			continue
		}
		if err := s.releaseWatchQuota(ctx, candidate.BucketID, false); err != nil {
			return err
		}
		if _, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0, not_before_ms=0
			WHERE id=? AND run_id=? AND status='scheduled'`, candidate.BucketID, candidate.RunID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) ReserveWatchQuota(ctx context.Context, bucketID, userID int64, taskIDs []int64, dayStartMs, hourStartMs int64, dailyLimit, hourlyLimit int) (bool, int64, error) {
	if s.conn != nil {
		var allowed bool
		var retryAtMs int64
		err := s.Transact(ctx, func(ctx context.Context, tx Store) error {
			var err error
			allowed, retryAtMs, err = tx.ReserveWatchQuota(ctx, bucketID, userID, taskIDs, dayStartMs, hourStartMs, dailyLimit, hourlyLimit)
			return err
		})
		return allowed, retryAtMs, err
	}
	if bucketID <= 0 || userID <= 0 || dailyLimit <= 0 || hourlyLimit <= 0 {
		return false, 0, fmt.Errorf("invalid watch quota reservation")
	}
	var bucketRow struct {
		Status string `db:"status"`
	}
	if err := s.exec.QueryRowCtx(ctx, &bucketRow, `SELECT status FROM watch_delivery_bucket
		WHERE id=? AND user_id=? FOR UPDATE`, bucketID, userID); err != nil {
		if err == sqlx.ErrNotFound {
			return false, 0, nil
		}
		return false, 0, err
	}
	if bucketRow.Status != "pending" && bucketRow.Status != "deferred" {
		return false, 0, nil
	}
	var existing int64
	if err := s.exec.QueryRowCtx(ctx, &existing, `SELECT COUNT(*) FROM watch_send_reservation WHERE bucket_id=?`, bucketID); err != nil {
		return false, 0, err
	}
	if existing > 0 {
		return true, 0, nil
	}
	sort.Slice(taskIDs, func(i, j int) bool { return taskIDs[i] < taskIDs[j] })
	uniqueTasks := taskIDs[:0]
	for _, taskID := range taskIDs {
		if taskID <= 0 || (len(uniqueTasks) > 0 && uniqueTasks[len(uniqueTasks)-1] == taskID) {
			continue
		}
		uniqueTasks = append(uniqueTasks, taskID)
	}
	type quotaRow struct {
		taskID int64
		kind   string
		start  int64
		limit  int
		retry  int64
	}
	quotas := []quotaRow{{taskID: 0, kind: "day", start: dayStartMs, limit: dailyLimit, retry: dayStartMs + int64((24 * time.Hour).Milliseconds())}}
	for _, taskID := range uniqueTasks {
		quotas = append(quotas, quotaRow{taskID: taskID, kind: "hour", start: hourStartMs, limit: hourlyLimit, retry: hourStartMs + int64(time.Hour.Milliseconds())})
	}
	for _, quota := range quotas {
		if _, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_send_stat
			(user_id, task_id, period_kind, period_start_ms, sent_count, reserved_count) VALUES (?, ?, ?, ?, 0, 0)
			ON DUPLICATE KEY UPDATE sent_count=sent_count`,
			userID, quota.taskID, quota.kind, quota.start); err != nil {
			return false, 0, err
		}
		var row struct {
			Sent     int64 `db:"sent_count"`
			Reserved int64 `db:"reserved_count"`
		}
		if err := s.exec.QueryRowCtx(ctx, &row, `SELECT sent_count, reserved_count FROM watch_send_stat
			WHERE user_id=? AND task_id=? AND period_kind=? AND period_start_ms=? FOR UPDATE`,
			userID, quota.taskID, quota.kind, quota.start); err != nil {
			return false, 0, err
		}
		if row.Sent+row.Reserved >= int64(quota.limit) {
			return false, quota.retry, nil
		}
	}
	for _, quota := range quotas {
		if _, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_send_reservation
			(bucket_id, user_id, task_id, period_kind, period_start_ms, created_at_ms) VALUES (?, ?, ?, ?, ?, ?)`,
			bucketID, userID, quota.taskID, quota.kind, quota.start, NowMs()); err != nil {
			return false, 0, err
		}
		res, err := s.exec.ExecCtx(ctx, `UPDATE watch_send_stat SET reserved_count=reserved_count+1
			WHERE user_id=? AND task_id=? AND period_kind=? AND period_start_ms=?`, userID, quota.taskID, quota.kind, quota.start)
		if err != nil {
			return false, 0, err
		}
		affected, err := res.RowsAffected()
		if err != nil || affected != 1 {
			if err != nil {
				return false, 0, err
			}
			return false, 0, fmt.Errorf("watch quota reservation stat missing")
		}
	}
	return true, 0, nil
}

func (s *SQLStore) FinishWatchDelivery(ctx context.Context, id, userID, runID int64, delivered bool, nowMs int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error {
			return tx.FinishWatchDelivery(ctx, id, userID, runID, delivered, nowMs)
		})
	}
	bucket, err := s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, not_before_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE id=? AND user_id=? AND run_id=? FOR UPDATE`, id, userID, runID)
	if err != nil {
		return err
	}
	if bucket == nil {
		// A user Stop/foreground message may have already returned this bucket
		// to pending. That is a valid terminal cleanup outcome for the run.
		return nil
	}
	if bucket.Status == "sent" {
		return nil
	}
	if bucket.Status == "pending" || bucket.Status == "deferred" || bucket.Status == "discarded" {
		return s.releaseWatchQuota(ctx, id, false)
	}
	if bucket.Status != "scheduled" {
		return fmt.Errorf("watch bucket %d is not scheduled", id)
	}
	if !delivered {
		if err := s.releaseWatchQuota(ctx, id, false); err != nil {
			return err
		}
		_, err = s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0
			WHERE id=? AND user_id=? AND run_id=? AND status='scheduled'`, id, userID, runID)
		return err
	}
	res, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='sent'
		WHERE id=? AND user_id=? AND run_id=? AND status='scheduled'`, id, userID, runID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("watch bucket %d delivery CAS failed", id)
	}
	var reservationCount int64
	if err := s.exec.QueryRowCtx(ctx, &reservationCount, `SELECT COUNT(*) FROM watch_send_reservation WHERE bucket_id=?`, id); err != nil {
		return err
	}
	if reservationCount > 0 {
		return s.releaseWatchQuota(ctx, id, true)
	}
	return s.commitLegacyWatchQuota(ctx, bucket, nowMs)
}

func (s *SQLStore) ResetUnsentBuckets(ctx context.Context, userID int64) error {
	if s.conn != nil {
		return s.Transact(ctx, func(ctx context.Context, tx Store) error { return tx.ResetUnsentBuckets(ctx, userID) })
	}
	var bucketIDs []int64
	if err := s.exec.QueryRowsCtx(ctx, &bucketIDs, `SELECT id FROM watch_delivery_bucket
		WHERE user_id=? AND status='scheduled' ORDER BY id FOR UPDATE`, userID); err != nil {
		return err
	}
	for _, bucketID := range bucketIDs {
		if err := s.releaseWatchQuota(ctx, bucketID, false); err != nil {
			return err
		}
		if _, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0, not_before_ms=0
			WHERE id=? AND status='scheduled'`, bucketID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) releaseWatchQuota(ctx context.Context, bucketID int64, delivered bool) error {
	var rows []struct {
		UserID        int64  `db:"user_id"`
		TaskID        int64  `db:"task_id"`
		PeriodKind    string `db:"period_kind"`
		PeriodStartMs int64  `db:"period_start_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT user_id, task_id, period_kind, period_start_ms
		FROM watch_send_reservation WHERE bucket_id=? ORDER BY period_kind, task_id FOR UPDATE`, bucketID); err != nil {
		return err
	}
	for _, row := range rows {
		query := `UPDATE watch_send_stat SET reserved_count=reserved_count-1 WHERE
			user_id=? AND task_id=? AND period_kind=? AND period_start_ms=? AND reserved_count>0`
		if delivered {
			query = `UPDATE watch_send_stat SET reserved_count=reserved_count-1, sent_count=sent_count+1 WHERE
				user_id=? AND task_id=? AND period_kind=? AND period_start_ms=? AND reserved_count>0`
		}
		res, err := s.exec.ExecCtx(ctx, query, row.UserID, row.TaskID, row.PeriodKind, row.PeriodStartMs)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil || affected != 1 {
			if err != nil {
				return err
			}
			return fmt.Errorf("watch quota reservation is inconsistent for bucket %d", bucketID)
		}
	}
	_, err := s.exec.ExecCtx(ctx, `DELETE FROM watch_send_reservation WHERE bucket_id=?`, bucketID)
	return err
}

func (s *SQLStore) commitLegacyWatchQuota(ctx context.Context, bucket *DeliveryBucket, nowMs int64) error {
	if bucket == nil {
		return nil
	}
	dayStart := nowMs / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	if err := s.IncrSent(ctx, bucket.UserID, 0, "day", dayStart); err != nil {
		return err
	}
	if len(bucket.HitIDs) == 0 {
		return nil
	}
	var taskIDs []int64
	args := append([]any{bucket.UserID}, intsToAny(bucket.HitIDs)...)
	if err := s.exec.QueryRowsCtx(ctx, &taskIDs, `SELECT DISTINCT task_id FROM watch_hit
		WHERE user_id=? AND id IN (`+placeholders(len(bucket.HitIDs))+`)`, args...); err != nil {
		return err
	}
	hourStart := nowMs / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
	for _, taskID := range taskIDs {
		if err := s.IncrSent(ctx, bucket.UserID, taskID, "hour", hourStart); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) CountSent(ctx context.Context, userID, taskID int64, periodKind string, periodStartMs int64) (int, error) {
	var row struct {
		N int64 `db:"n"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT IFNULL(sent_count,0) AS n FROM watch_send_stat
		WHERE user_id=? AND task_id=? AND period_kind=? AND period_start_ms=?`, userID, taskID, periodKind, periodStartMs); err != nil {
		if err == sqlx.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return int(row.N), nil
}

func (s *SQLStore) IncrSent(ctx context.Context, userID, taskID int64, periodKind string, periodStartMs int64) error {
	_, err := s.exec.ExecCtx(ctx, `INSERT INTO watch_send_stat (user_id, task_id, period_kind, period_start_ms, sent_count)
		VALUES (?, ?, ?, ?, 1)
		ON DUPLICATE KEY UPDATE sent_count = sent_count + 1`, userID, taskID, periodKind, periodStartMs)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullBytes(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.Repeat("?,", n-1) + "?"
}

func intsToAny(ids []int64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func stringsToAny(values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func decodeInt64s(raw []byte) []int64 {
	if len(raw) == 0 {
		return nil
	}
	var out []int64
	_ = json.Unmarshal(raw, &out)
	return out
}

func reverseMessages(rows []Message) {
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
}
