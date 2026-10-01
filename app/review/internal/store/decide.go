package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"esx/pkg/event"
	"esx/pkg/idempotencyx"
	"esx/pkg/mqx"
	"esx/pkg/outboxx"
	sqlx "esx/pkg/sqlstore"

	"github.com/go-sql-driver/mysql"
)

// MachineLease 是 worker 处理单个任务的租约；崩溃后到期即可被其他 worker 重领。
const MachineLease = time.Minute

// DecisionInput 描述一次结论。
type DecisionInput struct {
	Verdict       string
	PolicyCodes   []string
	PolicyVersion string
	Source        string
	DecidedBy     int64
	Note          string
	// 拒绝且提名时生成候选种子（RVW-030）
	NominateSeed bool
	SeedText     string
	// 机审自动通过时创建质检任务（RVW-014）
	CreateQA bool
}

// ClaimMachine 领取一个待机审任务；machine_running 且租约过期的任务视同待领取。
func (s *Store) ClaimMachine(ctx context.Context, now time.Time) (*Task, error) {
	var claimed *Task
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		var task Task
		err := session.QueryRowCtx(ctx, &task, `SELECT `+taskColumns+` FROM review_task
			WHERE status = ? OR (status = ? AND lease_until_ms < ?)
			ORDER BY priority DESC, id ASC LIMIT 1 FOR UPDATE SKIP LOCKED`,
			StatusMachinePending, StatusMachineRunning, now.UnixMilli())
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		task.Status = StatusMachineRunning
		task.LeaseGeneration++
		task.LeaseUntilMs = now.Add(MachineLease).UnixMilli()
		task.UpdatedAtMs = now.UnixMilli()
		if _, err := session.ExecCtx(ctx, `UPDATE review_task SET status = ?, lease_generation = ?,
			lease_until_ms = ?, updated_at_ms = ? WHERE id = ?`,
			task.Status, task.LeaseGeneration, task.LeaseUntilMs, task.UpdatedAtMs, task.ID); err != nil {
			return err
		}
		claimed = &task
		return nil
	})
	return claimed, err
}

// Escalate 把机审中的任务转人审，并记录原因与优先级（RVW-011、RVW-013）。
func (s *Store) Escalate(ctx context.Context, task *Task, reason string, priority int, now time.Time) error {
	result, err := s.conn.ExecCtx(ctx, `UPDATE review_task SET status = ?, escalation_reason = ?, priority = ?,
		lease_holder = 0, lease_until_ms = 0, updated_at_ms = ?
		WHERE id = ? AND status = ? AND lease_generation = ?`,
		StatusHumanPending, truncate(reason, 64), priority, now.UnixMilli(),
		task.ID, StatusMachineRunning, task.LeaseGeneration)
	if err != nil {
		return err
	}
	return s.expectFenced(ctx, result, task.ID)
}

// DecideMachine 以机审结论关闭任务；fencing 依赖机审租约代次。
func (s *Store) DecideMachine(ctx context.Context, task *Task, in DecisionInput, qaDeadlineMs int64, now time.Time) (*Decision, error) {
	var out *Decision
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		locked, err := s.lockTask(ctx, session, task.ID)
		if err != nil {
			return err
		}
		if locked.Status != StatusMachineRunning || locked.LeaseGeneration != task.LeaseGeneration {
			return fencingError(locked)
		}
		decision, err := s.writeDecision(ctx, session, locked, in, now)
		if err != nil {
			return err
		}
		if in.CreateQA {
			if _, err := s.insertFollowUp(ctx, session, locked, event.ReviewPurposeQA, 0, 0, qaDeadlineMs, now); err != nil {
				return err
			}
		}
		out = decision
		return nil
	})
	return out, err
}

// RescanPauseReason 是回扫机审判定违规、已暂停投放并等待人审的转人审原因（ADS-031）。
const RescanPauseReason = "rescan-violation"

// FlagRescanViolation 处理回扫机审判定违规（ADS-031）：同一事务内把任务转人审，并经 outbox 下发
// 暂停结论（Interim），业务方先停投；最终结论由该任务的人审给出，确认违规则下线，否定则恢复。
// fencing 依赖机审租约代次，与 DecideMachine 一致。
func (s *Store) FlagRescanViolation(ctx context.Context, task *Task, codes []string, policyVersion string, priority int, now time.Time) error {
	return s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		locked, err := s.lockTask(ctx, session, task.ID)
		if err != nil {
			return err
		}
		if locked.Purpose != event.ReviewPurposeRescan {
			return fmt.Errorf("review: task %d is not a rescan", task.ID)
		}
		if locked.Status != StatusMachineRunning || locked.LeaseGeneration != task.LeaseGeneration {
			return fencingError(locked)
		}
		if _, err := session.ExecCtx(ctx, `UPDATE review_task SET status = ?, escalation_reason = ?, priority = ?,
			lease_holder = 0, lease_until_ms = 0, updated_at_ms = ? WHERE id = ?`,
			StatusHumanPending, RescanPauseReason, priority, now.UnixMilli(), locked.ID); err != nil {
			return err
		}
		id, err := s.nextID()
		if err != nil {
			return err
		}
		interim := event.ReviewDecidedEvent{
			EventID: id, EventTime: now.UnixMilli(), BizType: locked.BizType, ObjectID: locked.ObjectID,
			Revision: locked.ObjectRevision, TaskID: locked.ID, Purpose: locked.Purpose, PurposeKey: locked.PurposeKey,
			Verdict: event.ReviewVerdictReject, PolicyCodes: codes, PolicyVersion: policyVersion,
			Source: event.ReviewSourceMachine, DecidedAt: now.UnixMilli(), Interim: true,
		}
		if err := interim.Validate(); err != nil {
			return err
		}
		payload, err := interim.MarshalPayload()
		if err != nil {
			return err
		}
		if err := s.outbox.Enqueue(ctx, session, outboxx.Event{
			ID: id, Topic: mqx.TopicReviewDecided, Tag: locked.BizType,
			Key: "review-interim:" + strconv.FormatInt(locked.ID, 10), Payload: payload,
		}); err != nil {
			return err
		}
		return s.insertAudit(ctx, session, AuditEntry{
			ActorID: 0, Action: "task.rescan_pause", ObjectType: "review_task", ObjectID: locked.ID,
			Before: taskState(locked),
			After:  map[string]any{"status": StatusHumanPending, "policyCodes": codes, "policyVersion": policyVersion},
		}, now)
	})
}

// SubmitHuman 提交人工结论（RVW-021）：校验持有者、代次与租约，同事务写结论、事件与审计。
// 同一幂等键重复提交返回首次结论；网络重试不会产生第二条结论。
func (s *Store) SubmitHuman(
	ctx context.Context,
	reviewerID, taskID, generation int64,
	idempotencyKey string,
	in DecisionInput,
	now time.Time,
) (*Decision, error) {
	var out *Decision
	err := s.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		recordID, err := s.nextID()
		if err != nil {
			return err
		}
		decisionID, err := s.nextID()
		if err != nil {
			return err
		}
		rec := idempotencyx.IdempotencyRecord{
			Scope: "review:decision", UserID: reviewerID, Key: idempotencyKey,
			CommandHash: idempotencyx.CommandHash(strconv.FormatInt(taskID, 10), strconv.FormatInt(generation, 10),
				in.Verdict, encodeCodes(in.PolicyCodes)),
		}
		resourceID, created, err := idempotencyx.ResolveIdempotencySession(ctx, session, rec, recordID, decisionID)
		if err != nil {
			return err
		}
		if !created {
			var existing Decision
			if err := session.QueryRowCtx(ctx, &existing, `SELECT `+decisionColumns+` FROM review_decision WHERE id = ?`, resourceID); err != nil {
				return err
			}
			out = &existing
			return nil
		}
		locked, err := s.lockTask(ctx, session, taskID)
		if err != nil {
			return err
		}
		if !holdsLease(locked, reviewerID, generation, now) {
			return fencingError(locked)
		}
		in.DecidedBy = reviewerID
		decision, err := s.writeDecisionWithID(ctx, session, decisionID, locked, in, now)
		if err != nil {
			return err
		}
		after := *locked
		after.Status = StatusDecided
		if err := s.insertAudit(ctx, session, AuditEntry{
			ActorID: reviewerID, Action: "task.submit", ObjectType: "review_task", ObjectID: taskID,
			Before: taskState(locked),
			After: map[string]any{"status": StatusDecided, "verdict": in.Verdict, "policyCodes": in.PolicyCodes,
				"decisionId": decision.ID, "leaseGeneration": generation},
		}, now); err != nil {
			return err
		}
		if in.NominateSeed && in.Verdict == event.ReviewVerdictReject && len(in.PolicyCodes) > 0 && in.SeedText != "" {
			if err := s.nominateSeed(ctx, session, locked, in.PolicyCodes[0], in.SeedText, reviewerID, now); err != nil {
				return err
			}
		}
		out = decision
		return nil
	})
	return out, err
}

func (s *Store) writeDecision(ctx context.Context, session sqlx.Session, task *Task, in DecisionInput, now time.Time) (*Decision, error) {
	id, err := s.nextID()
	if err != nil {
		return nil, err
	}
	return s.writeDecisionWithID(ctx, session, id, task, in, now)
}

// writeDecisionWithID 写结论、关闭任务、更新指纹复用、写结论事件（RVW-004、RVW-015、RVW-040）。
func (s *Store) writeDecisionWithID(ctx context.Context, session sqlx.Session, id int64, task *Task, in DecisionInput, now time.Time) (*Decision, error) {
	decision := &Decision{
		ID: id, TaskID: task.ID, BizType: task.BizType, ObjectID: task.ObjectID, ObjectRevision: task.ObjectRevision,
		Purpose: task.Purpose, Verdict: in.Verdict, PolicyCodesJSON: encodeCodes(in.PolicyCodes),
		PolicyVersion: in.PolicyVersion, Source: in.Source, DecidedBy: in.DecidedBy,
		LeaseGeneration: task.LeaseGeneration, Note: truncate(in.Note, 500), DecidedAtMs: now.UnixMilli(),
	}
	if _, err := session.ExecCtx(ctx, `INSERT INTO review_decision (`+decisionColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		decision.ID, decision.TaskID, decision.BizType, decision.ObjectID, decision.ObjectRevision, decision.Purpose,
		decision.Verdict, decision.PolicyCodesJSON, decision.PolicyVersion, decision.Source, decision.DecidedBy,
		decision.LeaseGeneration, decision.Note, decision.DecidedAtMs); err != nil {
		return nil, err
	}
	if _, err := session.ExecCtx(ctx, `UPDATE review_task SET status = ?, decision_id = ?, decided_at_ms = ?,
		lease_until_ms = 0, updated_at_ms = ? WHERE id = ?`,
		StatusDecided, decision.ID, decision.DecidedAtMs, now.UnixMilli(), task.ID); err != nil {
		return nil, err
	}
	if err := s.rememberVerdict(ctx, session, task, decision, now); err != nil {
		return nil, err
	}
	if decision.Verdict == event.ReviewVerdictApprove && slices.Contains(approvalPurposes, task.Purpose) {
		if err := supersedeReplacedServing(ctx, session, task.BizType, task.ObjectID, task.ObjectRevision, now); err != nil {
			return nil, err
		}
	}
	payload, err := event.ReviewDecidedEvent{
		EventID: decision.ID, EventTime: now.UnixMilli(), BizType: task.BizType, ObjectID: task.ObjectID,
		Revision: task.ObjectRevision, TaskID: task.ID, Purpose: task.Purpose, PurposeKey: task.PurposeKey,
		Verdict: decision.Verdict, PolicyCodes: decision.PolicyCodes(), PolicyVersion: decision.PolicyVersion,
		Source: decision.Source, DecidedAt: decision.DecidedAtMs,
	}.MarshalPayload()
	if err != nil {
		return nil, err
	}
	if err := s.outbox.Enqueue(ctx, session, outboxx.Event{
		ID: decision.ID, Topic: mqx.TopicReviewDecided, Tag: task.BizType,
		Key: "review-decided:" + strconv.FormatInt(task.ID, 10), Payload: payload,
	}); err != nil {
		return nil, err
	}
	return decision, nil
}

// rememberVerdict 维护指纹复用（RVW-015）：拒绝结论任何来源都可复用；通过结论只来自人工或质检。
// 首次审核、申诉与质检的结论是对快照本身的判断；回扫与举报结论另有语义，不写入。
func (s *Store) rememberVerdict(ctx context.Context, session sqlx.Session, task *Task, d *Decision, now time.Time) error {
	if task.Purpose != event.ReviewPurposeInitial && task.Purpose != event.ReviewPurposeAppeal && task.Purpose != event.ReviewPurposeQA {
		return nil
	}
	humanSourced := d.Source == event.ReviewSourceHuman || d.Source == event.ReviewSourceQA
	if d.Verdict == event.ReviewVerdictApprove && !humanSourced {
		return nil
	}
	if d.Verdict == event.ReviewVerdictApprove && humanSourced {
		if err := s.rememberApprovedMedia(ctx, session, task.SnapshotHash, d.ID, now); err != nil {
			return err
		}
	}
	_, err := session.ExecCtx(ctx, `INSERT INTO verdict_cache
		(snapshot_hash, market, policy_version, verdict, policy_codes, source, decision_id, created_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE verdict = VALUES(verdict), policy_codes = VALUES(policy_codes),
			source = VALUES(source), decision_id = VALUES(decision_id), created_at_ms = VALUES(created_at_ms)`,
		task.SnapshotHash, task.Market, d.PolicyVersion, d.Verdict, d.PolicyCodesJSON, d.Source, d.ID, now.UnixMilli())
	return err
}

func (s *Store) rememberApprovedMedia(ctx context.Context, session sqlx.Session, snapshotHash string, decisionID int64, now time.Time) error {
	var content string
	if err := session.QueryRowCtx(ctx, &content, `SELECT content FROM review_snapshot WHERE hash = ?`, snapshotHash); err != nil {
		return err
	}
	var frozen struct {
		Snapshot event.ReviewSnapshot `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(content), &frozen); err != nil {
		return err
	}
	for _, media := range frozen.Snapshot.Media {
		if media.SHA256 == "" {
			continue
		}
		if _, err := session.ExecCtx(ctx, `INSERT IGNORE INTO approved_media (sha256, decision_id, created_at_ms)
			VALUES (?, ?, ?)`, media.SHA256, decisionID, now.UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}

func holdsLease(task *Task, reviewerID, generation int64, now time.Time) bool {
	return task.Status == StatusClaimed && task.LeaseHolder == reviewerID &&
		task.LeaseGeneration == generation && task.LeaseUntilMs > now.UnixMilli()
}

// fencingError 区分「任务已作废」「已有结论」与「持有已失效」（RVW-003、RVW-021）。
func fencingError(task *Task) error {
	switch task.Status {
	case StatusSuperseded:
		return ErrTaskSuperseded
	case StatusDecided:
		return ErrTaskDecided
	default:
		return ErrLeaseLost
	}
}

func (s *Store) expectFenced(ctx context.Context, result interface{ RowsAffected() (int64, error) }, taskID int64) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	return fencingError(task)
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
