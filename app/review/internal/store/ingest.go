package store

import (
	"context"
	"errors"
	"time"

	"esx/app/review/internal/snapshot"
	"esx/pkg/event"
	sqlx "esx/pkg/sqlstore"
)

// IngestResult 是一次送审的结果。
type IngestResult struct {
	Task    *Task
	Created bool
}

// Ingest 冻结快照并按唯一键幂等建任务（RVW-001、RVW-002、RVW-003）。
//
// 新 revision 的任务在同一事务内把同对象旧 revision 的未决任务置为 superseded；
// 乱序到达的旧 revision 送审直接以 superseded 落库，保证不会被领取或下发结论。
func (s *Store) Ingest(
	ctx context.Context,
	sub event.ReviewSubmittedEvent,
	frozen snapshot.Frozen,
	policyVersion string,
	deadlineMs int64,
	now time.Time,
) (IngestResult, error) {
	var result IngestResult
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if _, err := session.ExecCtx(ctx, `INSERT IGNORE INTO review_snapshot (hash, biz_type, content, created_at)
			VALUES (?, ?, ?, ?)`, frozen.Hash, frozen.BizType, string(frozen.Content), now.UnixMilli()); err != nil {
			return err
		}
		existing, err := findTaskByKey(ctx, session, sub, true)
		if err != nil {
			return err
		}
		if existing != nil {
			result = IngestResult{Task: existing}
			return nil
		}
		var newest int64
		if err := session.QueryRowCtx(ctx, &newest, `SELECT COALESCE(MAX(object_revision), 0) FROM review_task
			WHERE biz_type = ? AND object_id = ? FOR UPDATE`, sub.BizType, sub.ObjectID); err != nil {
			return err
		}
		status := StatusMachinePending
		if newest > sub.Revision {
			status = StatusSuperseded
		} else if err := supersedeOlder(ctx, session, sub.BizType, sub.ObjectID, sub.Revision, now); err != nil {
			return err
		}
		seq := 0
		if sub.Purpose == event.ReviewPurposeInitial {
			if err := session.QueryRowCtx(ctx, &seq, `SELECT COUNT(*) FROM review_task
				WHERE biz_type = ? AND submitter_id = ? AND purpose = ?`,
				sub.BizType, frozen.Snapshot.SubmitterID, event.ReviewPurposeInitial); err != nil {
				return err
			}
			seq++
		}
		id, err := s.nextID()
		if err != nil {
			return err
		}
		task := &Task{
			ID: id, BizType: sub.BizType, ObjectID: sub.ObjectID, ObjectRevision: sub.Revision,
			Purpose: sub.Purpose, PurposeKey: sub.PurposeKey, SnapshotHash: frozen.Hash,
			Market: frozen.Snapshot.Market, Language: frozen.Snapshot.Language, Industry: frozen.Snapshot.Industry,
			SubmitterID: frozen.Snapshot.SubmitterID, SubmissionSeq: seq,
			RequiredRole: requiredRole(sub.BizType, sub.Purpose), Status: status,
			Priority: int(sub.Priority), DeadlineMs: deadlineMs, PolicyVersion: policyVersion,
			SubmittedAtMs: sub.SubmittedAt, CreatedAtMs: now.UnixMilli(), UpdatedAtMs: now.UnixMilli(),
		}
		if err := insertTask(ctx, session, task); err != nil {
			return err
		}
		result = IngestResult{Task: task, Created: true}
		return nil
	})
	if err != nil {
		// 并发同键：唯一键失败后回读胜出事务的任务（RVW-001）。
		if isDuplicateKey(err) {
			task, findErr := findTaskByKey(ctx, s.conn, sub, false)
			if findErr == nil && task != nil {
				return IngestResult{Task: task}, nil
			}
		}
		return IngestResult{}, err
	}
	return result, nil
}

// InsertFollowUpTask 在既有事务中为同一对象 revision 创建质检、申诉等后续任务。
func (s *Store) insertFollowUp(
	ctx context.Context,
	session sqlx.Session,
	source *Task,
	purpose string,
	excludedReviewer int64,
	priority int,
	deadlineMs int64,
	now time.Time,
) (*Task, error) {
	id, err := s.nextID()
	if err != nil {
		return nil, err
	}
	task := &Task{
		ID: id, BizType: source.BizType, ObjectID: source.ObjectID, ObjectRevision: source.ObjectRevision,
		Purpose: purpose, SnapshotHash: source.SnapshotHash, Market: source.Market, Language: source.Language,
		Industry: source.Industry, SubmitterID: source.SubmitterID,
		RequiredRole: requiredRole(source.BizType, purpose), Status: StatusHumanPending,
		Priority: priority, DeadlineMs: deadlineMs, ExcludedReviewer: excludedReviewer,
		SourceTaskID: source.ID, EscalationReason: purpose, PolicyVersion: source.PolicyVersion,
		SubmittedAtMs: now.UnixMilli(), CreatedAtMs: now.UnixMilli(), UpdatedAtMs: now.UnixMilli(),
	}
	if _, err := session.ExecCtx(ctx, `INSERT IGNORE INTO review_task (`+taskColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskArgs(task)...); err != nil {
		return nil, err
	}
	return task, nil
}

func requiredRole(bizType, purpose string) string {
	if purpose == event.ReviewPurposeQA {
		return RoleQA
	}
	if bizType == event.ReviewBizAdvertiserQualification {
		return RoleQualificationReviewer
	}
	return RoleReviewer
}

func supersedeOlder(ctx context.Context, session sqlx.Session, bizType string, objectID, revision int64, now time.Time) error {
	args := []any{StatusSuperseded, 0, 0, now.UnixMilli(), bizType, objectID, revision}
	args = append(args, anyArgs(pendingStatuses)...)
	_, err := session.ExecCtx(ctx, `UPDATE review_task
		SET status = ?, lease_holder = ?, lease_until_ms = ?, updated_at_ms = ?
		WHERE biz_type = ? AND object_id = ? AND object_revision < ? AND status IN (`+placeholders(len(pendingStatuses))+`)`,
		args...)
	return err
}

type rowQuerier interface {
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
}

func findTaskByKey(ctx context.Context, q rowQuerier, sub event.ReviewSubmittedEvent, lock bool) (*Task, error) {
	query := `SELECT ` + taskColumns + ` FROM review_task
		WHERE biz_type = ? AND object_id = ? AND object_revision = ? AND purpose = ? AND purpose_key = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var task Task
	err := q.QueryRowCtx(ctx, &task, query, sub.BizType, sub.ObjectID, sub.Revision, sub.Purpose, sub.PurposeKey)
	if errors.Is(err, sqlx.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func insertTask(ctx context.Context, session sqlx.Session, task *Task) error {
	_, err := session.ExecCtx(ctx, `INSERT INTO review_task (`+taskColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskArgs(task)...)
	return err
}

func taskArgs(t *Task) []any {
	return []any{
		t.ID, t.BizType, t.ObjectID, t.ObjectRevision, t.Purpose, t.PurposeKey, t.SnapshotHash,
		t.Market, t.Language, t.Industry, t.SubmitterID, t.SubmissionSeq, t.RequiredRole, t.Status, t.Priority,
		t.DeadlineMs, t.LeaseHolder, t.LeaseGeneration, t.LeaseUntilMs, t.Attempts, t.ExcludedReviewer,
		t.SourceTaskID, t.EscalationReason, t.PolicyVersion, t.DecisionID, t.SubmittedAtMs, t.DecidedAtMs,
		t.CreatedAtMs, t.UpdatedAtMs,
	}
}
