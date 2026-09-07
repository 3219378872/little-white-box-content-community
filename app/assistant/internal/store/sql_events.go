package store

import (
	"context"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

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
