package logic

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"esx/app/review/internal/intake"
	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
	"esx/pkg/errx"
	"esx/pkg/event"
	"esx/pkg/idempotencyx"

	"esx/pkg/logging"
)

// base 是各审核 Logic 共用的请求上下文、依赖与日志。
type base struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// newBase 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func newBase(ctx context.Context, svcCtx *svc.ServiceContext) base {
	return base{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// reviewer 每次请求实时读取角色（RVW-050）：撤销对下一次请求立即生效。
func (b base) reviewer(userID int64, anyOf ...string) (*store.Reviewer, error) {
	if userID <= 0 {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	r, err := b.svcCtx.Store.Reviewer(b.ctx, userID)
	if errors.Is(err, store.ErrReviewerUnknown) {
		return nil, errx.NewWithCode(errx.ReviewRoleRequired)
	}
	if err != nil {
		b.Errorw("load reviewer failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if !r.Active {
		return nil, errx.NewWithCode(errx.ReviewRoleRequired)
	}
	if len(anyOf) == 0 {
		return r, nil
	}
	for _, role := range anyOf {
		if r.HasRole(role) {
			return r, nil
		}
	}
	return nil, errx.NewWithCode(errx.ReviewRoleRequired)
}

// canSeeTask：具备任务所需角色且在授权市场与语言内；资质对象只对资质审核员可见（RVW-023）。
func canSeeTask(r *store.Reviewer, task *store.Task) bool {
	if r == nil || !r.Active || !r.HasRole(task.RequiredRole) {
		return false
	}
	return slices.Contains(r.Markets(), task.Market) && slices.Contains(r.Languages(), task.Language)
}

// mapStoreError 把租约、围栏、幂等与送审校验错误映射为业务错误码，未识别的错误记日志后返回系统错误。
func (b base) mapStoreError(err error, action string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrLeaseLost):
		return errx.NewWithCode(errx.ReviewLeaseLost)
	case errors.Is(err, store.ErrTaskSuperseded):
		return errx.NewWithCode(errx.ReviewTaskSuperseded)
	case errors.Is(err, store.ErrTaskDecided):
		return errx.NewWithCode(errx.ReviewTaskDecided)
	case errors.Is(err, store.ErrTaskNotFound), errors.Is(err, store.ErrSeedNotFound):
		return errx.NewWithCode(errx.NotFound)
	case errors.Is(err, store.ErrSameActor):
		return errx.NewWithCode(errx.PermissionDenied)
	case errors.Is(err, store.ErrSeedTransition):
		return errx.NewWithCode(errx.ParamError)
	case errors.Is(err, idempotencyx.ErrIdempotencyConflict):
		return errx.NewWithCode(errx.IdempotencyConflict)
	case errors.As(err, new(intake.ErrInvalidSubmission)):
		return errx.NewWithCode(errx.ParamError)
	default:
		b.Errorw(action+" failed", logging.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
}

// taskView 组装任务视图；withEvidence 时附带冻结快照与各阶段记录，只给有权查看的审核员。
func (b base) taskView(task *store.Task, withEvidence bool) (*pb.TaskView, error) {
	view := &pb.TaskView{
		TaskId: task.ID, BizType: task.BizType, ObjectId: task.ObjectID, ObjectRevision: task.ObjectRevision,
		Purpose: task.Purpose, Status: task.Status, Market: task.Market, Language: task.Language,
		Industry: task.Industry, Priority: int32(task.Priority), DeadlineMs: task.DeadlineMs,
		LeaseGeneration: task.LeaseGeneration, LeaseUntilMs: task.LeaseUntilMs, SubmittedAtMs: task.SubmittedAtMs,
		EscalationReason: task.EscalationReason, PolicyVersion: task.PolicyVersion, Attempts: int32(task.Attempts),
	}
	if !withEvidence {
		return view, nil
	}
	content, err := b.svcCtx.Store.Snapshot(b.ctx, task.SnapshotHash)
	if err != nil {
		return nil, err
	}
	view.SnapshotJson = content
	stages, err := b.svcCtx.Store.Stages(b.ctx, task.ID)
	if err != nil {
		return nil, err
	}
	for _, stage := range stages {
		view.Stages = append(view.Stages, &pb.StageView{
			Stage: stage.Stage, ComponentVersion: stage.ComponentVersion, Shadow: stage.Shadow,
			Outcome: stage.Outcome, Reason: stage.Reason, OutputJson: stage.Output, LatencyMs: stage.LatencyMs,
		})
	}
	// 质检与申诉展示原结论（FX-113）；原任务阶段记录随之展示，便于复核机审依据。
	if task.SourceTaskID > 0 {
		original, err := b.svcCtx.Store.DecisionByTask(b.ctx, task.SourceTaskID)
		if err != nil {
			return nil, err
		}
		view.OriginalDecision = decisionView(original)
		sourceStages, err := b.svcCtx.Store.Stages(b.ctx, task.SourceTaskID)
		if err != nil {
			return nil, err
		}
		for _, stage := range sourceStages {
			view.Stages = append(view.Stages, &pb.StageView{
				Stage: stage.Stage, ComponentVersion: stage.ComponentVersion, Shadow: stage.Shadow,
				Outcome: stage.Outcome, Reason: stage.Reason, OutputJson: stage.Output, LatencyMs: stage.LatencyMs,
			})
		}
	}
	if task.DecisionID > 0 {
		decision, err := b.svcCtx.Store.DecisionByTask(b.ctx, task.ID)
		if err != nil {
			return nil, err
		}
		view.Decision = decisionView(decision)
	}
	return view, nil
}

// decisionView 组装结论视图。
func decisionView(d *store.Decision) *pb.DecisionView {
	if d == nil {
		return nil
	}
	return &pb.DecisionView{
		DecisionId: d.ID, Verdict: d.Verdict, PolicyCodes: d.PolicyCodes(), PolicyVersion: d.PolicyVersion,
		Source: d.Source, DecidedAtMs: d.DecidedAtMs,
	}
}

// snapshotOf 解码冻结快照。
func snapshotOf(content string) (event.ReviewSnapshot, error) {
	var frozen struct {
		Snapshot event.ReviewSnapshot `json:"snapshot"`
	}
	err := json.Unmarshal([]byte(content), &frozen)
	return frozen.Snapshot, err
}
