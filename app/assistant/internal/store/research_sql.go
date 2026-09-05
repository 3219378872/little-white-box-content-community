package store

import (
	"context"
	"encoding/json"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func (s *SQLStore) LockRun(ctx context.Context, id int64) (*Run, error) {
	return s.scanRun(ctx, runSelect+" WHERE id=? FOR UPDATE", id)
}

func (s *SQLStore) ListSourceEvents(ctx context.Context, runID int64) ([]Event, error) {
	var rows []struct {
		ID          int64  `db:"id"`
		Seq         int64  `db:"seq"`
		Payload     []byte `db:"payload_json"`
		CreatedAtMs int64  `db:"created_at_ms"`
	}
	err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT id,seq,payload_json,created_at_ms FROM agent_run_event WHERE run_id=? AND type='source_card' ORDER BY seq DESC LIMIT 100`, runID)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		out = append(out, Event{ID: row.ID, RunID: runID, Seq: row.Seq, Type: EventSourceCard, PayloadJSON: row.Payload, CreatedAtMs: row.CreatedAtMs})
	}
	return out, nil
}

func (s *SQLStore) HasDeletedRunHistory(ctx context.Context, run Run) (bool, error) {
	var row struct {
		N int `db:"n"`
	}
	err := s.exec.QueryRowCtx(ctx, &row, `SELECT COUNT(*) AS n FROM assistant_message m
		WHERE m.user_id=? AND m.deleted_at_ms IS NOT NULL AND
		(m.run_id=? OR m.id=(SELECT message_id FROM assistant_input_command WHERE user_id=? AND request_id=? LIMIT 1))`, run.UserID, run.ID, run.UserID, run.RequestID)
	return row.N > 0, err
}

func (s *SQLStore) ListWaitingRuns(ctx context.Context) ([]Run, error) {
	var rows []runRow
	err := s.exec.QueryRowsCtx(ctx, &rows, runSelect+" WHERE status='waiting_input' ORDER BY last_activity_at_ms, id LIMIT 100")
	out := make([]Run, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRun())
	}
	return out, err
}

func (s *SQLStore) PutEvidence(ctx context.Context, item Evidence) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = s.exec.ExecCtx(ctx, `INSERT IGNORE INTO agent_source_evidence
		(run_id,handle,evidence_id,payload_json,created_at_ms) VALUES (?,?,?,?,?)`,
		item.RunID, item.Handle, item.ID, string(raw), item.RetrievedAtMs)
	return err
}

func (s *SQLStore) ListEvidence(ctx context.Context, runID int64, handle string) ([]Evidence, error) {
	var rows []struct {
		Payload []byte `db:"payload_json"`
	}
	err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT payload_json FROM agent_source_evidence WHERE run_id=? AND handle=? ORDER BY evidence_id`, runID, handle)
	if err != nil {
		return nil, err
	}
	out := make([]Evidence, 0, len(rows))
	for _, row := range rows {
		var item Evidence
		if err := json.Unmarshal(row.Payload, &item); err != nil {
			return nil, err
		}
		item.RunID = runID
		out = append(out, item)
	}
	return out, nil
}

func (s *SQLStore) SaveQuestion(ctx context.Context, item QuestionRequest) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = s.exec.ExecCtx(ctx, `INSERT INTO agent_question_request
		(id,run_id,user_id,message_id,payload_json,answer_request_id,answer_digest) VALUES (?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE payload_json=VALUES(payload_json),answer_request_id=VALUES(answer_request_id),answer_digest=VALUES(answer_digest)`,
		item.ID, item.RunID, item.UserID, item.MessageID, string(raw), item.AnswerRequestID, item.AnswerDigest)
	return err
}

func (s *SQLStore) ListQuestions(ctx context.Context, runID int64) ([]QuestionRequest, error) {
	var rows []struct {
		UserID    int64  `db:"user_id"`
		Payload   []byte `db:"payload_json"`
		RequestID string `db:"answer_request_id"`
		Digest    string `db:"answer_digest"`
	}
	err := s.exec.QueryRowsCtx(ctx, &rows, `SELECT user_id,payload_json,answer_request_id,answer_digest
		FROM agent_question_request WHERE run_id=? ORDER BY message_id`, runID)
	if err != nil {
		return nil, err
	}
	out := make([]QuestionRequest, 0, len(rows))
	for _, row := range rows {
		var item QuestionRequest
		if err := json.Unmarshal(row.Payload, &item); err != nil {
			return nil, err
		}
		item.UserID, item.AnswerRequestID, item.AnswerDigest = row.UserID, row.RequestID, row.Digest
		out = append(out, item)
	}
	return out, nil
}

func (s *SQLStore) SavePresentation(ctx context.Context, item AnswerPresentation) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = s.exec.ExecCtx(ctx, `INSERT INTO assistant_message_presentation (message_id,payload_json) VALUES (?,?)`, item.MessageID, string(raw))
	return err
}

func (s *SQLStore) GetPresentation(ctx context.Context, messageID int64) (*AnswerPresentation, error) {
	var row struct {
		Payload []byte `db:"payload_json"`
	}
	err := s.exec.QueryRowCtx(ctx, &row, `SELECT payload_json FROM assistant_message_presentation WHERE message_id=?`, messageID)
	if err == sqlx.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var item AnswerPresentation
	if err := json.Unmarshal(row.Payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *SQLStore) ClearResearchHistory(ctx context.Context, userID int64) error {
	for _, query := range []string{
		`DELETE FROM agent_question_request WHERE user_id=?`,
		`DELETE p FROM assistant_message_presentation p JOIN assistant_message m ON m.id=p.message_id WHERE m.user_id=?`,
		`DELETE e FROM agent_source_evidence e JOIN agent_run r ON r.id=e.run_id WHERE r.user_id=?`,
		`DELETE s FROM agent_source_ledger s JOIN agent_run r ON r.id=s.run_id WHERE r.user_id=?`,
	} {
		if _, err := s.exec.ExecCtx(ctx, query, userID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) PurgeExpiredSourceEvidence(ctx context.Context, cutoffMs int64, batchSize int) (int, error) {
	batchSize = boundedPurgeBatchSize(batchSize)
	result, err := s.exec.ExecCtx(ctx, `DELETE FROM agent_source_evidence WHERE created_at_ms<? ORDER BY created_at_ms LIMIT ?`, cutoffMs, batchSize)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count < int64(batchSize) {
		result, err = s.exec.ExecCtx(ctx, `DELETE FROM agent_source_ledger WHERE created_at_ms<? ORDER BY created_at_ms LIMIT ?`, cutoffMs, batchSize-int(count))
		if err != nil {
			return int(count), err
		}
		n, err := result.RowsAffected()
		return int(count + n), err
	}
	return int(count), nil
}
