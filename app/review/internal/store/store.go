// Package store 是审核平台的权威存储（xbh_review）。
//
// 任务状态机：machine_pending → machine_running → decided | human_pending；
// human_pending → claimed → decided | human_pending；任一未决状态 → superseded。
// 人审持有使用（task, reviewer, lease_generation）fencing；结论、任务更新、
// 结论事件与审计在同一事务内提交（RVW-021、RVW-025、RVW-040）。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"esx/pkg/outboxx"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/util"
)

const (
	StatusMachinePending = "machine_pending"
	StatusMachineRunning = "machine_running"
	StatusHumanPending   = "human_pending"
	StatusClaimed        = "claimed"
	StatusDecided        = "decided"
	StatusSuperseded     = "superseded"
)

// 角色（RVW-050）。
const (
	RoleReviewer              = "reviewer"
	RoleQA                    = "qa"
	RolePolicyAdmin           = "policy_admin"
	RoleQualificationReviewer = "qualification_reviewer"
)

// HumanLease 是人审持有期限（RVW-020）。
const HumanLease = 10 * time.Minute

// MaxHumanAttempts 超过后提升优先级并告警，避免无限重领（DES-review-platform）。
const MaxHumanAttempts = 5

var (
	ErrLeaseLost       = errors.New("review: lease is no longer held")
	ErrTaskSuperseded  = errors.New("review: task superseded")
	ErrTaskDecided     = errors.New("review: task already decided")
	ErrTaskNotFound    = errors.New("review: task not found")
	ErrReviewerUnknown = errors.New("review: reviewer not found")
	ErrSeedNotFound    = errors.New("review: seed not found")
	ErrSeedTransition  = errors.New("review: seed transition not allowed")
	ErrSameActor       = errors.New("review: nominator and confirmer must differ")
)

var pendingStatuses = []string{StatusMachinePending, StatusMachineRunning, StatusHumanPending, StatusClaimed}

// Task 是 review_task 一行。
type Task struct {
	ID               int64  `db:"id"`
	BizType          string `db:"biz_type"`
	ObjectID         int64  `db:"object_id"`
	ObjectRevision   int64  `db:"object_revision"`
	Purpose          string `db:"purpose"`
	PurposeKey       string `db:"purpose_key"`
	SnapshotHash     string `db:"snapshot_hash"`
	Market           string `db:"market"`
	Language         string `db:"language"`
	Industry         string `db:"industry"`
	SubmitterID      int64  `db:"submitter_id"`
	SubmissionSeq    int    `db:"submission_seq"`
	RequiredRole     string `db:"required_role"`
	Status           string `db:"status"`
	Priority         int    `db:"priority"`
	DeadlineMs       int64  `db:"deadline_ms"`
	LeaseHolder      int64  `db:"lease_holder"`
	LeaseGeneration  int64  `db:"lease_generation"`
	LeaseUntilMs     int64  `db:"lease_until_ms"`
	Attempts         int    `db:"attempts"`
	ExcludedReviewer int64  `db:"excluded_reviewer"`
	SourceTaskID     int64  `db:"source_task_id"`
	EscalationReason string `db:"escalation_reason"`
	PolicyVersion    string `db:"policy_version"`
	DecisionID       int64  `db:"decision_id"`
	SubmittedAtMs    int64  `db:"submitted_at_ms"`
	DecidedAtMs      int64  `db:"decided_at_ms"`
	CreatedAtMs      int64  `db:"created_at_ms"`
	UpdatedAtMs      int64  `db:"updated_at_ms"`
}

const taskColumns = `id, biz_type, object_id, object_revision, purpose, purpose_key, snapshot_hash,
	market, language, industry, submitter_id, submission_seq, required_role, status, priority,
	deadline_ms, lease_holder, lease_generation, lease_until_ms, attempts, excluded_reviewer,
	source_task_id, escalation_reason, policy_version, decision_id, submitted_at_ms, decided_at_ms,
	created_at_ms, updated_at_ms`

// Decision 是 review_decision 一行。
type Decision struct {
	ID              int64  `db:"id"`
	TaskID          int64  `db:"task_id"`
	BizType         string `db:"biz_type"`
	ObjectID        int64  `db:"object_id"`
	ObjectRevision  int64  `db:"object_revision"`
	Purpose         string `db:"purpose"`
	Verdict         string `db:"verdict"`
	PolicyCodesJSON string `db:"policy_codes"`
	PolicyVersion   string `db:"policy_version"`
	Source          string `db:"source"`
	DecidedBy       int64  `db:"decided_by"`
	LeaseGeneration int64  `db:"lease_generation"`
	Note            string `db:"note"`
	DecidedAtMs     int64  `db:"decided_at_ms"`
}

const decisionColumns = `id, task_id, biz_type, object_id, object_revision, purpose, verdict,
	policy_codes, policy_version, source, decided_by, lease_generation, note, decided_at_ms`

// PolicyCodes 解码政策码列表。
func (d Decision) PolicyCodes() []string {
	var codes []string
	if err := json.Unmarshal([]byte(d.PolicyCodesJSON), &codes); err != nil {
		return nil
	}
	return codes
}

// Stage 是 review_stage_result 一行（RVW-017）。
type Stage struct {
	ID               int64  `db:"id"`
	TaskID           int64  `db:"task_id"`
	Stage            string `db:"stage"`
	ComponentVersion string `db:"component_version"`
	Shadow           bool   `db:"shadow"`
	Outcome          string `db:"outcome"`
	Reason           string `db:"reason"`
	Output           string `db:"output"`
	LatencyMs        int64  `db:"latency_ms"`
	CreatedAtMs      int64  `db:"created_at_ms"`
}

// Reviewer 是 reviewer 一行，每次请求实时读取（RVW-050）。
type Reviewer struct {
	UserID      int64  `db:"user_id"`
	RolesCSV    string `db:"roles"`
	MarketsCSV  string `db:"markets"`
	LanguageCSV string `db:"languages"`
	Active      bool   `db:"active"`
	UpdatedAtMs int64  `db:"updated_at_ms"`
}

func (r Reviewer) Roles() []string     { return splitCSV(r.RolesCSV) }
func (r Reviewer) Markets() []string   { return splitCSV(r.MarketsCSV) }
func (r Reviewer) Languages() []string { return splitCSV(r.LanguageCSV) }

// HasRole 报告有效审核员是否具备角色。
func (r *Reviewer) HasRole(role string) bool {
	return r != nil && r.Active && slices.Contains(r.Roles(), role)
}

// Store 封装 xbh_review 的全部读写。
type Store struct {
	conn   sqlx.SqlConn
	outbox *outboxx.SQLStore
	nextID func() (int64, error)
}

func New(conn sqlx.SqlConn) *Store {
	return &Store{conn: conn, outbox: outboxx.NewSQLStore(conn), nextID: util.NextID}
}

// AuditEntry 是一条只追加审计（RVW-025）。
type AuditEntry struct {
	ActorID    int64
	Action     string
	ObjectType string
	ObjectID   int64
	Before     any
	After      any
}

func (s *Store) insertAudit(ctx context.Context, session sqlx.Session, entry AuditEntry, now time.Time) error {
	id, err := s.nextID()
	if err != nil {
		return err
	}
	_, err = session.ExecCtx(ctx, `INSERT INTO audit_log
		(id, actor_id, action, object_type, object_id, before_state, after_state, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, entry.ActorID, entry.Action, entry.ObjectType, entry.ObjectID,
		auditJSON(entry.Before), auditJSON(entry.After), now.UnixMilli())
	return err
}

// Audit 在独立事务外写一条审计，用于政策激活等非事务动作。
func (s *Store) Audit(ctx context.Context, entry AuditEntry, now time.Time) error {
	return s.insertAudit(ctx, s.conn, entry, now)
}

func auditJSON(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%q", fmt.Sprint(value))
	}
	if len(raw) > 2000 {
		return string(raw[:2000])
	}
	return string(raw)
}

func taskState(t *Task) map[string]any {
	if t == nil {
		return nil
	}
	return map[string]any{
		"status": t.Status, "leaseHolder": t.LeaseHolder,
		"leaseGeneration": t.LeaseGeneration, "leaseUntilMs": t.LeaseUntilMs,
	}
}

func (s *Store) lockTask(ctx context.Context, session sqlx.Session, id int64) (*Task, error) {
	var task Task
	err := session.QueryRowCtx(ctx, &task, `SELECT `+taskColumns+` FROM review_task WHERE id = ? FOR UPDATE`, id)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// GetTask 读取任务。
func (s *Store) GetTask(ctx context.Context, id int64) (*Task, error) {
	var task Task
	err := s.conn.QueryRowCtx(ctx, &task, `SELECT `+taskColumns+` FROM review_task WHERE id = ?`, id)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Snapshot 读取冻结快照内容。
func (s *Store) Snapshot(ctx context.Context, hash string) (string, error) {
	var content string
	err := s.conn.QueryRowCtx(ctx, &content, `SELECT content FROM review_snapshot WHERE hash = ?`, hash)
	return content, err
}

// Stages 读取任务的全部阶段记录。
func (s *Store) Stages(ctx context.Context, taskID int64) ([]Stage, error) {
	var rows []Stage
	err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT id, task_id, stage, component_version, shadow, outcome,
		reason, output, latency_ms, created_at_ms FROM review_stage_result WHERE task_id = ? ORDER BY id`, taskID)
	return rows, err
}

// DecisionByTask 读取任务结论；没有结论时返回 nil。
func (s *Store) DecisionByTask(ctx context.Context, taskID int64) (*Decision, error) {
	var d Decision
	err := s.conn.QueryRowCtx(ctx, &d, `SELECT `+decisionColumns+` FROM review_decision WHERE task_id = ?`, taskID)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Reviewer 实时读取审核员；不存在时返回 ErrReviewerUnknown。
func (s *Store) Reviewer(ctx context.Context, userID int64) (*Reviewer, error) {
	var r Reviewer
	err := s.conn.QueryRowCtx(ctx, &r, `SELECT user_id, roles, markets, languages, active, updated_at_ms
		FROM reviewer WHERE user_id = ?`, userID)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, ErrReviewerUnknown
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RecordStage 幂等写入阶段记录：（task, stage, version, shadow）重复时保留首条（DES 失败模式）。
func (s *Store) RecordStage(ctx context.Context, stage Stage, now time.Time) error {
	id, err := s.nextID()
	if err != nil {
		return err
	}
	_, err = s.conn.ExecCtx(ctx, `INSERT IGNORE INTO review_stage_result
		(id, task_id, stage, component_version, shadow, outcome, reason, output, latency_ms, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, stage.TaskID, stage.Stage, stage.ComponentVersion, stage.Shadow, stage.Outcome,
		stage.Reason, stage.Output, stage.LatencyMs, now.UnixMilli())
	return err
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func encodeCodes(codes []string) string {
	if codes == nil {
		codes = []string{}
	}
	raw, _ := json.Marshal(codes)
	return string(raw)
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anyArgs[T any](values []T) []any {
	out := make([]any, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
