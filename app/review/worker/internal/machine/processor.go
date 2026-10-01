// Package machine 执行单个机审任务：运行级联、记录阶段、写结论或转人审，并按需运行影子政策。
package machine

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"esx/app/review/internal/cascade"
	"esx/app/review/internal/metrics"
	"esx/app/review/internal/policy"
	"esx/app/review/internal/store"
	"esx/pkg/event"

	logx "esx/pkg/logging"
)

// Store 是机审需要的存储操作。
type Store interface {
	Snapshot(ctx context.Context, hash string) (string, error)
	RecordStage(ctx context.Context, stage store.Stage, now time.Time) error
	DecideMachine(ctx context.Context, task *store.Task, in store.DecisionInput, qaDeadlineMs int64, now time.Time) (*store.Decision, error)
	Escalate(ctx context.Context, task *store.Task, reason string, priority int, now time.Time) error
}

// Processor 处理已领取的机审任务。
type Processor struct {
	Store   Store
	Cascade *cascade.Cascade
	Active  *policy.Policy
	Shadow  *policy.Policy
	Clock   func() time.Time
	// Sample 返回是否抽中质检；默认按政策比例随机抽样（RVW-014）。
	Sample func(rate float64) bool
}

func (p *Processor) now() time.Time {
	if p.Clock != nil {
		return p.Clock()
	}
	return time.Now()
}

func (p *Processor) sample(rate float64) bool {
	if p.Sample != nil {
		return p.Sample(rate)
	}
	return rand.Float64() < rate
}

// Process 运行级联并落结论。fencing 失败（任务已作废或已被接手）时丢弃本次结果。
func (p *Processor) Process(ctx context.Context, task *store.Task) error {
	content, err := p.Store.Snapshot(ctx, task.SnapshotHash)
	if err != nil {
		return err
	}
	snap, err := decodeSnapshot(content)
	if err != nil {
		// 快照不可解码属于数据损坏，转人审而不是无限重试。
		return p.Store.Escalate(ctx, task, "snapshot-invalid", 90, p.now())
	}
	input := cascade.Input{
		TaskID: task.ID, BizType: task.BizType, Purpose: task.Purpose,
		SnapshotHash: task.SnapshotHash, Snapshot: snap, SubmissionSeq: task.SubmissionSeq,
	}
	result := p.Cascade.Run(ctx, p.Active, input)
	if err := p.recordStages(ctx, task.ID, result.Stages, false); err != nil {
		return err
	}
	if p.Shadow != nil {
		p.runShadow(ctx, task.ID, input, result)
	}
	now := p.now()
	switch result.Outcome {
	case cascade.OutcomeApprove, cascade.OutcomeReject:
		verdict := event.ReviewVerdictApprove
		if result.Outcome == cascade.OutcomeReject {
			verdict = event.ReviewVerdictReject
		}
		createQA := verdict == event.ReviewVerdictApprove && task.Purpose == event.ReviewPurposeInitial &&
			p.sample(p.Active.QASampleRate)
		decision, err := p.Store.DecideMachine(ctx, task, store.DecisionInput{
			Verdict: verdict, PolicyCodes: result.PolicyCodes, PolicyVersion: p.Active.Version,
			Source: event.ReviewSourceMachine, Note: result.Reason, CreateQA: createQA,
		}, p.Active.DeadlineMs(now.UnixMilli()), now)
		if err != nil {
			return fenced(ctx, err)
		}
		metrics.Decision(decision.Source, decision.Verdict, task.Purpose, task.SubmittedAtMs, now)
	default:
		if err := p.Store.Escalate(ctx, task, result.Reason, result.Priority, now); err != nil {
			return fenced(ctx, err)
		}
	}
	return nil
}

// runShadow 用影子政策跑同一快照，只落阶段记录（RVW-016）；任何失败都不影响生效路径。
func (p *Processor) runShadow(ctx context.Context, taskID int64, input cascade.Input, active cascade.Result) {
	shadow := p.Cascade.Run(ctx, p.Shadow, input)
	if err := p.recordStages(ctx, taskID, shadow.Stages, true); err != nil {
		logx.WithContext(ctx).Errorw("record shadow stages failed", logx.Field("taskId", taskID), logx.Field("err", err.Error()))
	}
	metrics.Shadow(shadow.Outcome, shadow.Outcome == active.Outcome)
}

func (p *Processor) recordStages(ctx context.Context, taskID int64, stages []cascade.StageRecord, shadow bool) error {
	now := p.now()
	for _, stage := range stages {
		if err := p.Store.RecordStage(ctx, store.Stage{
			TaskID: taskID, Stage: stage.Stage, ComponentVersion: stage.ComponentVersion, Shadow: shadow,
			Outcome: stage.Outcome, Reason: stage.Reason, Output: stage.Output, LatencyMs: stage.LatencyMs,
		}, now); err != nil {
			return err
		}
		if shadow {
			continue
		}
		metrics.Stage(stage.Stage, stage.LatencyMs)
		if stage.Outcome == "degraded" {
			metrics.Degraded(stage.Stage, stage.Reason)
		}
		if stage.Stage == cascade.StageFingerprint {
			metrics.Fingerprint(stage.Outcome)
		}
	}
	return nil
}

func fenced(ctx context.Context, err error) error {
	if errors.Is(err, store.ErrTaskSuperseded) || errors.Is(err, store.ErrLeaseLost) || errors.Is(err, store.ErrTaskDecided) {
		logx.WithContext(ctx).Infow("machine review result dropped by fencing", logx.Field("err", err.Error()))
		return nil
	}
	return err
}
