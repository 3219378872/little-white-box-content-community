package store

import (
	"context"
	"errors"
	"slices"
	"time"

	sqlx "esx/pkg/sqlstore"
)

// ClaimHuman 按审核员授权领取下一单（RVW-020、RVW-022）。
//
// 只领取其角色、市场与语言范围内的任务，并排除自己作出原结论的质检与申诉（RVW-024）；
// 队列按优先级与截止时间排序，同等条件下先送审先处理。持有已过期的 claimed 任务视同待领取。
func (s *Store) ClaimHuman(ctx context.Context, reviewer *Reviewer, purpose string, now time.Time) (*Task, error) {
	roles := claimableRoles(reviewer)
	markets, languages := reviewer.Markets(), reviewer.Languages()
	if len(roles) == 0 || len(markets) == 0 || len(languages) == 0 {
		return nil, nil
	}
	var claimed *Task
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		query := `SELECT ` + taskColumns + ` FROM review_task
			WHERE (status = ? OR (status = ? AND lease_until_ms < ?))
			AND required_role IN (` + placeholders(len(roles)) + `)
			AND market IN (` + placeholders(len(markets)) + `)
			AND language IN (` + placeholders(len(languages)) + `)
			AND excluded_reviewer <> ?`
		args := []any{StatusHumanPending, StatusClaimed, now.UnixMilli()}
		args = append(args, anyArgs(roles)...)
		args = append(args, anyArgs(markets)...)
		args = append(args, anyArgs(languages)...)
		args = append(args, reviewer.UserID)
		if purpose != "" {
			query += ` AND purpose = ?`
			args = append(args, purpose)
		}
		query += ` ORDER BY priority DESC, deadline_ms ASC, id ASC LIMIT 1 FOR UPDATE SKIP LOCKED`
		var task Task
		err := session.QueryRowCtx(ctx, &task, query, args...)
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		before := task
		task.Status = StatusClaimed
		task.LeaseHolder = reviewer.UserID
		task.LeaseGeneration++
		task.LeaseUntilMs = now.Add(HumanLease).UnixMilli()
		task.Attempts++
		if task.Attempts > MaxHumanAttempts && task.Priority < 1000 {
			task.Priority += 100
		}
		task.UpdatedAtMs = now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `UPDATE review_task SET status = ?, lease_holder = ?, lease_generation = ?,
			lease_until_ms = ?, attempts = ?, priority = ?, updated_at_ms = ? WHERE id = ?`,
			task.Status, task.LeaseHolder, task.LeaseGeneration, task.LeaseUntilMs, task.Attempts,
			task.Priority, task.UpdatedAtMs, task.ID); err != nil {
			return err
		}
		if err := s.insertAudit(ctx, session, AuditEntry{
			ActorID: reviewer.UserID, Action: "task.claim", ObjectType: "review_task", ObjectID: task.ID,
			Before: taskState(&before), After: taskState(&task),
		}, now); err != nil {
			return err
		}
		claimed = &task
		return nil
	})
	return claimed, err
}

// Renew 在有效持有内续期 10 分钟（RVW-020、RVW-021）。
func (s *Store) Renew(ctx context.Context, reviewerID, taskID, generation int64, now time.Time) (*Task, error) {
	var renewed *Task
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		task, err := s.lockTask(ctx, session, taskID)
		if err != nil {
			return err
		}
		if !holdsLease(task, reviewerID, generation, now) {
			return fencingError(task)
		}
		before := *task
		task.LeaseUntilMs = now.Add(HumanLease).UnixMilli()
		task.UpdatedAtMs = now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `UPDATE review_task SET lease_until_ms = ?, updated_at_ms = ? WHERE id = ?`,
			task.LeaseUntilMs, task.UpdatedAtMs, task.ID); err != nil {
			return err
		}
		if err := s.insertAudit(ctx, session, AuditEntry{
			ActorID: reviewerID, Action: "task.renew", ObjectType: "review_task", ObjectID: taskID,
			Before: taskState(&before), After: taskState(task),
		}, now); err != nil {
			return err
		}
		renewed = task
		return nil
	})
	return renewed, err
}

// Release 放弃持有，任务回到队列（RVW-021）。
func (s *Store) Release(ctx context.Context, reviewerID, taskID, generation int64, now time.Time) error {
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		task, err := s.lockTask(ctx, session, taskID)
		if err != nil {
			return err
		}
		if !holdsLease(task, reviewerID, generation, now) {
			return fencingError(task)
		}
		before := *task
		task.Status = StatusHumanPending
		task.LeaseHolder = 0
		task.LeaseUntilMs = 0
		if _, err := session.ExecCtx(ctx, `UPDATE review_task SET status = ?, lease_holder = 0, lease_until_ms = 0,
			updated_at_ms = ? WHERE id = ?`, StatusHumanPending, now.UnixMilli(), taskID); err != nil {
			return err
		}
		return s.insertAudit(ctx, session, AuditEntry{
			ActorID: reviewerID, Action: "task.release", ObjectType: "review_task", ObjectID: taskID,
			Before: taskState(&before), After: taskState(task),
		}, now)
	})
}

// QueueBucket 是某目的的积压概况（RVW-060）。
type QueueBucket struct {
	Purpose      string `db:"purpose"`
	Pending      int64  `db:"pending"`
	OldestSubmit int64  `db:"oldest_submit"`
	OldestAgeMs  int64  `db:"-"`
}

// QueueSummary 返回审核员授权范围内的人审积压。
func (s *Store) QueueSummary(ctx context.Context, reviewer *Reviewer, now time.Time) ([]QueueBucket, error) {
	roles := claimableRoles(reviewer)
	markets, languages := reviewer.Markets(), reviewer.Languages()
	if len(roles) == 0 || len(markets) == 0 || len(languages) == 0 {
		return nil, nil
	}
	args := []any{StatusHumanPending, StatusClaimed, now.UnixMilli()}
	args = append(args, anyArgs(roles)...)
	args = append(args, anyArgs(markets)...)
	args = append(args, anyArgs(languages)...)
	args = append(args, reviewer.UserID)
	var rows []QueueBucket
	err := s.conn.QueryRowsCtx(ctx, &rows, `SELECT purpose, COUNT(*) AS pending, MIN(submitted_at_ms) AS oldest_submit
		FROM review_task
		WHERE (status = ? OR (status = ? AND lease_until_ms < ?))
		AND required_role IN (`+placeholders(len(roles))+`)
		AND market IN (`+placeholders(len(markets))+`)
		AND language IN (`+placeholders(len(languages))+`)
		AND excluded_reviewer <> ?
		GROUP BY purpose ORDER BY purpose`, args...)
	for i := range rows {
		rows[i].OldestAgeMs = max(0, now.UnixMilli()-rows[i].OldestSubmit)
	}
	return rows, err
}

// Backlog 是全局人审积压，用于指标（RVW-060）。
type Backlog struct {
	Pending      int64 `db:"pending"`
	OldestSubmit int64 `db:"oldest_submit"`
}

func (s *Store) HumanBacklog(ctx context.Context) (Backlog, error) {
	var b Backlog
	err := s.conn.QueryRowCtx(ctx, &b, `SELECT COUNT(*) AS pending, COALESCE(MIN(submitted_at_ms), 0) AS oldest_submit
		FROM review_task WHERE status IN (?, ?)`, StatusHumanPending, StatusClaimed)
	return b, err
}

func claimableRoles(reviewer *Reviewer) []string {
	if reviewer == nil || !reviewer.Active {
		return nil
	}
	var roles []string
	for _, role := range []string{RoleReviewer, RoleQA, RoleQualificationReviewer} {
		if slices.Contains(reviewer.Roles(), role) {
			roles = append(roles, role)
		}
	}
	return roles
}
