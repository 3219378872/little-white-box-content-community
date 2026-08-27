package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ToolAudit struct {
	Tool      string
	Status    string
	ArgDigest string
}

type RunRecord struct {
	UserID         int64
	RequestID      string
	ConversationID string
	Intent         string
	Model          string
	LatencyMS      int
	Status         string
	Tools          []ToolAudit
}

type AuditStore interface {
	Record(ctx context.Context, rec RunRecord) error
}

func digestArgs(argsJSON string) string {
	sum := sha256.Sum256([]byte(argsJSON))
	return hex.EncodeToString(sum[:])
}

func toolAuditStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func recordToolAudit(session *Session, name, argsJSON string, err error) {
	if session == nil {
		return
	}
	session.toolAudits = append(session.toolAudits, ToolAudit{
		Tool:      name,
		Status:    toolAuditStatus(err),
		ArgDigest: digestArgs(argsJSON),
	})
}

type MapAuditStore struct {
	mu    sync.Mutex
	Runs  []RunRecord
	Calls []ToolAudit
}

func NewMapAuditStore() *MapAuditStore {
	return &MapAuditStore{}
}

func (m *MapAuditStore) Record(_ context.Context, rec RunRecord) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := rec
	copied.Tools = append([]ToolAudit(nil), rec.Tools...)
	m.Runs = append(m.Runs, copied)
	m.Calls = append(m.Calls, copied.Tools...)
	return nil
}

func (m *MapAuditStore) Snapshot() []RunRecord {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RunRecord, len(m.Runs))
	copy(out, m.Runs)
	return out
}

type SQLAuditStore struct {
	conn sqlx.SqlConn
}

func NewSQLAuditStore(conn sqlx.SqlConn) *SQLAuditStore {
	if conn == nil {
		return nil
	}
	return &SQLAuditStore{conn: conn}
}

func (s *SQLAuditStore) Record(ctx context.Context, rec RunRecord) error {
	if s == nil || s.conn == nil {
		return nil
	}
	if rec.UserID <= 0 || strings.TrimSpace(rec.RequestID) == "" {
		return nil
	}
	res, err := s.conn.ExecCtx(ctx, `
		INSERT INTO agent_run (user_id, request_id, conversation_id, intent, model, latency_ms, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		  id = LAST_INSERT_ID(id),
		  intent = VALUES(intent),
		  model = VALUES(model),
		  latency_ms = VALUES(latency_ms),
		  status = VALUES(status)`,
		rec.UserID, rec.RequestID, rec.ConversationID, rec.Intent, rec.Model, rec.LatencyMS, rec.Status)
	if err != nil {
		return err
	}
	runID, err := res.LastInsertId()
	if err != nil || runID <= 0 {
		return err
	}
	for _, call := range rec.Tools {
		if strings.TrimSpace(call.Tool) == "" {
			continue
		}
		if _, err := s.conn.ExecCtx(ctx, `
			INSERT INTO tool_call (run_id, tool, status, arg_digest)
			VALUES (?, ?, ?, ?)`,
			runID, call.Tool, call.Status, call.ArgDigest); err != nil {
			return err
		}
	}
	return nil
}
