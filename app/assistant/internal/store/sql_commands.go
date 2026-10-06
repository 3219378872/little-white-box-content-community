package store

import (
	"context"
	"database/sql"
	"strings"

	sqlx "esx/pkg/sqlstore"
)

// InsertToolCall records a tool invocation requested by the model.
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

// GetToolCall reads one tool call; nil when it does not exist.
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

// UpdateToolCall stores the call's outcome.
func (s *SQLStore) UpdateToolCall(ctx context.Context, call ToolCall) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_tool_call SET status=?, result_json=? WHERE run_id=? AND call_id=?`,
		call.Status, nullString(call.ResultJSON), call.RunID, call.CallID)
	return err
}

// ListToolCalls returns a run's tool calls in creation order.
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

// GetJournal looks up the journal entry for one request, tool and argument digest; nil when absent.
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

// ReserveJournal claims the right to execute a side-effecting tool call. It returns (entry, true)
// when the caller should execute, and the existing entry with false when another attempt owns it.
func (s *SQLStore) ReserveJournal(ctx context.Context, row Journal) (*Journal, bool, error) {
	existing, err := s.GetJournal(ctx, row.UserID, row.RequestID, row.Tool, row.CanonicalArgsDigest)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		// An unfinished entry from an older lease is taken over; success is never re-executed.
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
		// A concurrent insert won the unique key; report its entry instead of executing twice.
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

// CompleteJournal records the final status and result of a reserved entry.
func (s *SQLStore) CompleteJournal(ctx context.Context, id int64, status, resultJSON string) error {
	_, err := s.exec.ExecCtx(ctx, `UPDATE agent_command_journal SET status=?, result_json=?, updated_at_ms=? WHERE id=?`,
		status, resultJSON, NowMs(), id)
	return err
}

// ListSuccessfulJournal returns side effects a request already completed; a run stopped at its
// resource limit reports them so the user knows what was done.
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

// InsertConfirmation records a tool call awaiting user approval.
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

// GetConfirmation reads the confirmation of one tool call; nil when absent.
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

// PendingConfirmation returns the oldest unresolved confirmation of a run.
func (s *SQLStore) PendingConfirmation(ctx context.Context, runID int64) (*Confirmation, error) {
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
	err := s.exec.QueryRowCtx(ctx, &row, `SELECT id, user_id, session_id, run_id, call_id, tool, canonical_args_digest, target_revision, status, created_at_ms, resolved_at_ms
		FROM agent_confirmation WHERE run_id=? AND status=? ORDER BY id LIMIT 1`, runID, ConfirmPending)
	if err == sqlx.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Confirmation{
		ID: row.ID, UserID: row.UserID, SessionID: row.SessionID, RunID: row.RunID, CallID: row.CallID, Tool: row.Tool,
		CanonicalArgsDigest: row.CanonicalArgsDigest, TargetRevision: row.TargetRevision, Status: row.Status,
		CreatedAtMs: row.CreatedAtMs, ResolvedAtMs: row.ResolvedAtMs.Int64,
	}, nil
}

// UpdateConfirmation resolves a confirmation only if it is still pending.
func (s *SQLStore) UpdateConfirmation(ctx context.Context, row Confirmation) error {
	res, err := s.exec.ExecCtx(ctx, `UPDATE agent_confirmation SET status=?, resolved_at_ms=? WHERE id=? AND status=?`,
		row.Status, row.ResolvedAtMs, row.ID, ConfirmPending)
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

// ResolveConfirmation applies the user's decision when user, call and argument digest all match
// a pending row, then returns the current row so repeated or stale answers see the recorded outcome.
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

// GetInputCommand finds the recorded outcome of a posted message; nil when absent.
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

// InsertInputCommand records how a posted message was handled; unique per user and request ID.
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

// CountQueue counts messages waiting behind a run, checked against MaxInputQueue.
func (s *SQLStore) CountQueue(ctx context.Context, runID int64) (int, error) {
	var row struct {
		N int64 `db:"n"`
	}
	if err := s.exec.QueryRowCtx(ctx, &row, `SELECT COUNT(*) AS n FROM agent_input_queue WHERE run_id=?`, runID); err != nil {
		return 0, err
	}
	return int(row.N), nil
}

// Enqueue parks a user message until the busy run picks it up.
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

// ListQueue returns queued messages in arrival order.
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

// DeleteQueueThrough removes queued items the run has consumed, up to maxID.
func (s *SQLStore) DeleteQueueThrough(ctx context.Context, runID, maxID int64) error {
	if maxID <= 0 {
		return nil
	}
	_, err := s.exec.ExecCtx(ctx, `DELETE FROM agent_input_queue WHERE run_id=? AND id<=?`, runID, maxID)
	return err
}

// DeleteQueue drops every queued item of a run being cancelled.
func (s *SQLStore) DeleteQueue(ctx context.Context, runID int64) error {
	_, err := s.exec.ExecCtx(ctx, `DELETE FROM agent_input_queue WHERE run_id=?`, runID)
	return err
}

// InsertAlert records a budget alert once per run, level and dimension; false when already raised.
func (s *SQLStore) InsertAlert(ctx context.Context, alert Alert) (bool, error) {
	res, err := s.exec.ExecCtx(ctx, `INSERT IGNORE INTO agent_run_alert (run_id, level, dimension, created_at_ms) VALUES (?, ?, ?, ?)`,
		alert.RunID, alert.Level, alert.Dimension, alert.CreatedAtMs)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
