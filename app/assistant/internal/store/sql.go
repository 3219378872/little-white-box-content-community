package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
		(user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, deleted_at_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.UserID, msg.SessionID, msg.RunID, msg.Role, msg.Kind, msg.Content, nullBytes(msg.APIContent),
		boolToInt(msg.Visible), boolToInt(msg.Unread), boolToInt(msg.Compacted), nullInt(msg.DeletedAtMs), msg.CreatedAtMs)
	if err != nil {
		return Message{}, err
	}
	id, _ := res.LastInsertId()
	msg.ID = id
	return msg, nil
}

func (s *SQLStore) GetMessage(ctx context.Context, userID, id int64) (*Message, error) {
	rows, err := s.scanMessages(ctx, `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sqlx.ErrNotFound
	}
	return &rows[0], nil
}

func (s *SQLStore) ListMessages(ctx context.Context, userID, sessionID, afterID int64, limit int) ([]Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, deleted_at_ms, created_at_ms
		FROM assistant_message WHERE user_id=? AND deleted_at_ms IS NULL AND visible=1`
	args := []any{userID}
	if sessionID > 0 {
		query += ` AND session_id=?`
		args = append(args, sessionID)
	}
	if afterID > 0 {
		query += ` AND id>?`
		args = append(args, afterID)
	}
	query += ` ORDER BY id ASC LIMIT ` + strconv.Itoa(limit)
	return s.scanMessages(ctx, query, args...)
}

func (s *SQLStore) ListSessionMessages(ctx context.Context, userID, sessionID int64, includeHidden bool) ([]Message, error) {
	query := `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, deleted_at_ms, created_at_ms
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
	query := `SELECT id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread, compacted, deleted_at_ms, created_at_ms
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
			Compacted: row.Compacted == 1, DeletedAtMs: row.DeletedAtMs.Int64, CreatedAtMs: row.CreatedAtMs,
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

func (s *SQLStore) InsertRun(ctx context.Context, run Run) (Run, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_run
		(user_id, session_id, request_id, source, status, phase, priority, queued_payload, lease_owner, lease_until_ms,
		 heartbeat_at_ms, cancel_requested, prompt_epoch, model, rounds, tool_calls, input_tokens, output_tokens, cache_tokens,
		 cost_usd, started_at_ms, ended_at_ms, last_activity_at_ms, error_code, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.UserID, run.SessionID, run.RequestID, run.Source, run.Status, run.Phase, run.Priority, nullBytes(run.QueuedPayload),
		nullString(run.LeaseOwner), nullInt(run.LeaseUntilMs), nullInt(run.HeartbeatAtMs), boolToInt(run.CancelRequested),
		run.PromptEpoch, nullString(run.Model), run.Rounds, run.ToolCalls, run.InputTokens, run.OutputTokens, run.CacheTokens,
		run.CostUSD, nullInt(run.StartedAtMs), nullInt(run.EndedAtMs), nullInt(run.LastActivityAtMs), nullString(run.ErrorCode), run.CreatedAtMs)
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
	return s.scanRun(ctx, `SELECT * FROM (`+runSelect+`) r WHERE r.user_id=? AND r.request_id=?`, userID, requestID)
}

const runSelect = `SELECT id, user_id, session_id, request_id, source, status, phase, priority, queued_payload, lease_owner,
	lease_until_ms, heartbeat_at_ms, cancel_requested, prompt_epoch, model, rounds, tool_calls, input_tokens, output_tokens,
	cache_tokens, cost_usd, started_at_ms, ended_at_ms, last_activity_at_ms, error_code, created_at_ms FROM agent_run`

func (s *SQLStore) scanRun(ctx context.Context, query string, args ...any) (*Run, error) {
	var row runRow
	if err := s.exec.QueryRowCtx(ctx, &row, query, args...); err != nil {
		return nil, err
	}
	out := row.toRun()
	return &out, nil
}

type runRow struct {
	ID               int64          `db:"id"`
	UserID           int64          `db:"user_id"`
	SessionID        int64          `db:"session_id"`
	RequestID        string         `db:"request_id"`
	Source           string         `db:"source"`
	Status           string         `db:"status"`
	Phase            string         `db:"phase"`
	Priority         int64          `db:"priority"`
	QueuedPayload    []byte         `db:"queued_payload"`
	LeaseOwner       sql.NullString `db:"lease_owner"`
	LeaseUntilMs     sql.NullInt64  `db:"lease_until_ms"`
	HeartbeatAtMs    sql.NullInt64  `db:"heartbeat_at_ms"`
	CancelRequested  int64          `db:"cancel_requested"`
	PromptEpoch      int64          `db:"prompt_epoch"`
	Model            sql.NullString `db:"model"`
	Rounds           int64          `db:"rounds"`
	ToolCalls        int64          `db:"tool_calls"`
	InputTokens      int64          `db:"input_tokens"`
	OutputTokens     int64          `db:"output_tokens"`
	CacheTokens      int64          `db:"cache_tokens"`
	CostUSD          float64        `db:"cost_usd"`
	StartedAtMs      sql.NullInt64  `db:"started_at_ms"`
	EndedAtMs        sql.NullInt64  `db:"ended_at_ms"`
	LastActivityAtMs sql.NullInt64  `db:"last_activity_at_ms"`
	ErrorCode        sql.NullString `db:"error_code"`
	CreatedAtMs      int64          `db:"created_at_ms"`
}

func (row runRow) toRun() Run {
	return Run{
		ID: row.ID, UserID: row.UserID, SessionID: row.SessionID, RequestID: row.RequestID, Source: row.Source,
		Status: row.Status, Phase: row.Phase, Priority: int(row.Priority), QueuedPayload: row.QueuedPayload,
		LeaseOwner: row.LeaseOwner.String, LeaseUntilMs: row.LeaseUntilMs.Int64, HeartbeatAtMs: row.HeartbeatAtMs.Int64,
		CancelRequested: row.CancelRequested == 1, PromptEpoch: int(row.PromptEpoch), Model: row.Model.String,
		Rounds: int(row.Rounds), ToolCalls: int(row.ToolCalls), InputTokens: row.InputTokens, OutputTokens: row.OutputTokens,
		CacheTokens: row.CacheTokens, CostUSD: row.CostUSD, StartedAtMs: row.StartedAtMs.Int64, EndedAtMs: row.EndedAtMs.Int64,
		LastActivityAtMs: row.LastActivityAtMs.Int64, ErrorCode: row.ErrorCode.String, CreatedAtMs: row.CreatedAtMs,
	}
}

func (s *SQLStore) UpdateRun(ctx context.Context, run Run) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET status=?, phase=?, queued_payload=?, lease_owner=?, lease_until_ms=?,
		heartbeat_at_ms=?, cancel_requested=cancel_requested OR ?, prompt_epoch=?, model=?, rounds=?, tool_calls=?, input_tokens=?, output_tokens=?,
		cache_tokens=?, cost_usd=?, started_at_ms=?, ended_at_ms=?, last_activity_at_ms=?, error_code=? WHERE id=?`,
		run.Status, run.Phase, nullBytes(run.QueuedPayload), nullString(run.LeaseOwner), nullInt(run.LeaseUntilMs),
		nullInt(run.HeartbeatAtMs), boolToInt(run.CancelRequested), run.PromptEpoch, nullString(run.Model), run.Rounds,
		run.ToolCalls, run.InputTokens, run.OutputTokens, run.CacheTokens, run.CostUSD, nullInt(run.StartedAtMs),
		nullInt(run.EndedAtMs), nullInt(run.LastActivityAtMs), nullString(run.ErrorCode), run.ID)
	return err
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

func (s *SQLStore) CancelOpenBackground(ctx context.Context, userID int64, sources []string) ([]Run, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	query := `SELECT * FROM (` + runSelect + `) r WHERE r.user_id=? AND r.status IN ('queued','running') AND r.source IN (` + placeholders(len(sources)) + `)`
	args := append([]any{userID}, stringsToAny(sources)...)
	var rows []runRow
	if err := s.exec.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		run := row.toRun()
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
		run.LeaseUntilMs = nowMs + leaseMs
		run.HeartbeatAtMs = nowMs
		run.LastActivityAtMs = nowMs
		if err := sqlTx.UpdateRun(ctx, run); err != nil {
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

func (s *SQLStore) RenewLease(ctx context.Context, runID int64, owner string, leaseUntilMs, heartbeatMs int64) (bool, error) {
	res, err := s.exec.ExecCtx(ctx, `UPDATE agent_run SET lease_until_ms=?, heartbeat_at_ms=? WHERE id=? AND lease_owner=? AND status='running'`,
		leaseUntilMs, heartbeatMs, runID, owner)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
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
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_run_event (run_id, seq, type, payload_json, created_at_ms) VALUES (?, ?, ?, ?, ?)`,
		runID, seq, eventType, nullBytes(payload), createdAtMs)
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
		ResultJSON          sql.NullString `db:"result_json"`
		Status              string         `db:"status"`
		CreatedAtMs         int64          `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, request_id, tool, canonical_args_digest, result_json, status, created_at_ms
		FROM agent_command_journal WHERE user_id=? AND request_id=? AND tool=? AND canonical_args_digest=?`,
		userID, requestID, tool, digest); err != nil {
		if err == sqlx.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &Journal{
		ID: row.ID, UserID: row.UserID, RequestID: row.RequestID, Tool: row.Tool, CanonicalArgsDigest: row.CanonicalArgsDigest,
		ResultJSON: row.ResultJSON.String, Status: row.Status, CreatedAtMs: row.CreatedAtMs,
	}, nil
}

func (s *SQLStore) ReserveJournal(ctx context.Context, row Journal) (*Journal, bool, error) {
	existing, err := s.GetJournal(ctx, row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, false, nil
	}
	res, err := s.exec.ExecCtx(ctx, `INSERT INTO agent_command_journal (user_id, request_id, tool, canonical_args_digest, result_json, status, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest, nullString(row.ResultJSON), row.Status, row.CreatedAtMs)
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
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_command_journal SET status=?, result_json=? WHERE id=?`, status, resultJSON, id)
	return err
}

func (s *SQLStore) ListSuccessfulJournal(ctx context.Context, userID int64, requestID string) ([]Journal, error) {
	var rows []struct {
		ID                  int64          `db:"id"`
		UserID              int64          `db:"user_id"`
		RequestID           string         `db:"request_id"`
		Tool                string         `db:"tool"`
		CanonicalArgsDigest string         `db:"canonical_args_digest"`
		ResultJSON          sql.NullString `db:"result_json"`
		Status              string         `db:"status"`
		CreatedAtMs         int64          `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, request_id, tool, canonical_args_digest, result_json, status, created_at_ms
		FROM agent_command_journal WHERE user_id=? AND request_id=? AND status=?`, userID, requestID, JournalSuccess); err != nil {
		return nil, err
	}
	out := make([]Journal, 0, len(rows))
	for _, row := range rows {
		out = append(out, Journal{
			ID: row.ID, UserID: row.UserID, RequestID: row.RequestID, Tool: row.Tool, CanonicalArgsDigest: row.CanonicalArgsDigest,
			ResultJSON: row.ResultJSON.String, Status: row.Status, CreatedAtMs: row.CreatedAtMs,
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
			Status        string `db:"status"`
			HitIDs        []byte `db:"hit_ids"`
			RunID         int64  `db:"run_id"`
			CreatedAtMs   int64  `db:"created_at_ms"`
		}
		if err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, window_start_ms, status, hit_ids, run_id, created_at_ms
			FROM watch_delivery_bucket WHERE user_id=? AND window_start_ms=?`, userID, windowStartMs); err != nil {
			return DeliveryBucket{}, err
		}
		existing = DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, Status: row.Status,
			HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs}
		return existing, nil
	}
	return DeliveryBucket{ID: id, UserID: userID, WindowStartMs: windowStartMs, Status: "pending", HitIDs: []int64{hitID}, CreatedAtMs: nowMs}, nil
}

func (s *SQLStore) GetBucket(ctx context.Context, id int64) (*DeliveryBucket, error) {
	return s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, status, hit_ids, run_id, created_at_ms FROM watch_delivery_bucket WHERE id=?`, id)
}

func (s *SQLStore) GetPendingBucket(ctx context.Context, userID int64) (*DeliveryBucket, error) {
	return s.scanBucket(ctx, `SELECT id, user_id, window_start_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE user_id=? AND status IN ('pending','deferred') ORDER BY window_start_ms ASC LIMIT 1`, userID)
}

func (s *SQLStore) ListDueBuckets(ctx context.Context, nowMs, windowMs int64) ([]DeliveryBucket, error) {
	var rows []struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
		Status        string `db:"status"`
		HitIDs        []byte `db:"hit_ids"`
		RunID         int64  `db:"run_id"`
		CreatedAtMs   int64  `db:"created_at_ms"`
	}
	if err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id, user_id, window_start_ms, status, hit_ids, run_id, created_at_ms
		FROM watch_delivery_bucket WHERE status IN ('pending','deferred') AND window_start_ms + ? <= ? ORDER BY window_start_ms ASC LIMIT 50`, windowMs, nowMs); err != nil {
		return nil, err
	}
	out := make([]DeliveryBucket, 0, len(rows))
	for _, row := range rows {
		out = append(out, DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, Status: row.Status,
			HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs})
	}
	return out, nil
}

func (s *SQLStore) scanBucket(ctx context.Context, query string, args ...any) (*DeliveryBucket, error) {
	var row struct {
		ID            int64  `db:"id"`
		UserID        int64  `db:"user_id"`
		WindowStartMs int64  `db:"window_start_ms"`
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
	return &DeliveryBucket{ID: row.ID, UserID: row.UserID, WindowStartMs: row.WindowStartMs, Status: row.Status,
		HitIDs: decodeInt64s(row.HitIDs), RunID: row.RunID, CreatedAtMs: row.CreatedAtMs}, nil
}

func (s *SQLStore) MarkBucketScheduled(ctx context.Context, id, runID int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='scheduled', run_id=? WHERE id=?`, runID, id)
	return err
}

func (s *SQLStore) MarkBucketSent(ctx context.Context, id int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='sent' WHERE id=?`, id)
	return err
}

func (s *SQLStore) ResetUnsentBuckets(ctx context.Context, userID int64) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE watch_delivery_bucket SET status='pending', run_id=0 WHERE user_id=? AND status IN ('scheduled')`, userID)
	return err
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
