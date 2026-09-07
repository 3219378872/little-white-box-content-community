package store

import (
	"context"
	"database/sql"
)

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
