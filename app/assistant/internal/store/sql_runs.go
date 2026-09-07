package store

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

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
