package logic

import (
	"context"
	"slices"
	"strings"

	"esx/app/review/internal/cascade"
	"esx/app/review/internal/metrics"
	"esx/app/review/internal/store"
	"esx/app/review/rpc/internal/svc"
	pb "esx/kitex_gen/review"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
	"esx/pkg/event"

	"esx/pkg/logging"
)

var claimRoles = []string{store.RoleReviewer, store.RoleQA, store.RoleQualificationReviewer}

// ClaimTaskLogic 承载 ClaimTask 接口的业务逻辑；每个请求新建一个实例。
type ClaimTaskLogic struct{ base }

// NewClaimTaskLogic 绑定请求上下文与服务依赖。
func NewClaimTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClaimTaskLogic {
	return &ClaimTaskLogic{newBase(ctx, svcCtx)}
}

// ClaimTask 领取授权范围内的下一单（RVW-020、RVW-022）。队列为空返回 found=false。
func (l *ClaimTaskLogic) ClaimTask(in *pb.ClaimTaskReq) (*pb.TaskResp, error) {
	r, err := l.reviewer(in.GetUserId(), claimRoles...)
	if err != nil {
		return nil, err
	}
	purpose := strings.TrimSpace(in.GetPurpose())
	if purpose != "" && !slices.Contains([]string{event.ReviewPurposeInitial, event.ReviewPurposeQA,
		event.ReviewPurposeAppeal, event.ReviewPurposeReport, event.ReviewPurposeRescan}, purpose) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	task, err := l.svcCtx.Store.ClaimHuman(l.ctx, r, purpose, l.svcCtx.Now())
	if err != nil {
		return nil, l.mapStoreError(err, "claim task")
	}
	if task == nil {
		return &pb.TaskResp{Found: false}, nil
	}
	if task.Attempts > store.MaxHumanAttempts {
		metrics.AttemptsExceeded()
		l.Infow("review task claimed beyond attempt limit", logging.Field("taskId", task.ID), logging.Field("attempts", task.Attempts))
	}
	view, err := l.taskView(task, true)
	if err != nil {
		return nil, l.mapStoreError(err, "load claimed task")
	}
	return &pb.TaskResp{Found: true, Task: view}, nil
}

// RenewTaskLogic 承载 RenewTask 接口的业务逻辑；每个请求新建一个实例。
type RenewTaskLogic struct{ base }

// NewRenewTaskLogic 绑定请求上下文与服务依赖。
func NewRenewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RenewTaskLogic {
	return &RenewTaskLogic{newBase(ctx, svcCtx)}
}

// RenewTask 续期当前租约；租约代际不符或已过期时返回租约丢失。
func (l *RenewTaskLogic) RenewTask(in *pb.LeaseReq) (*pb.TaskResp, error) {
	if _, err := l.reviewer(in.GetUserId(), claimRoles...); err != nil {
		return nil, err
	}
	task, err := l.svcCtx.Store.Renew(l.ctx, in.GetUserId(), in.GetTaskId(), in.GetLeaseGeneration(), l.svcCtx.Now())
	if err != nil {
		return nil, l.mapStoreError(err, "renew task")
	}
	view, err := l.taskView(task, false)
	if err != nil {
		return nil, l.mapStoreError(err, "load renewed task")
	}
	return &pb.TaskResp{Found: true, Task: view}, nil
}

// ReleaseTaskLogic 承载 ReleaseTask 接口的业务逻辑；每个请求新建一个实例。
type ReleaseTaskLogic struct{ base }

// NewReleaseTaskLogic 绑定请求上下文与服务依赖。
func NewReleaseTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReleaseTaskLogic {
	return &ReleaseTaskLogic{newBase(ctx, svcCtx)}
}

// ReleaseTask 主动释放租约，任务回到待领取队列。
func (l *ReleaseTaskLogic) ReleaseTask(in *pb.LeaseReq) (*pb.ReleaseTaskResp, error) {
	if _, err := l.reviewer(in.GetUserId(), claimRoles...); err != nil {
		return nil, err
	}
	if err := l.svcCtx.Store.Release(l.ctx, in.GetUserId(), in.GetTaskId(), in.GetLeaseGeneration(), l.svcCtx.Now()); err != nil {
		return nil, l.mapStoreError(err, "release task")
	}
	return &pb.ReleaseTaskResp{}, nil
}

// GetTaskLogic 承载 GetTask 接口的业务逻辑；每个请求新建一个实例。
type GetTaskLogic struct{ base }

// NewGetTaskLogic 绑定请求上下文与服务依赖。
func NewGetTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTaskLogic {
	return &GetTaskLogic{newBase(ctx, svcCtx)}
}

// GetTask 展示快照、机审证据与原结论（RVW-023）；越权统一返回不存在。
func (l *GetTaskLogic) GetTask(in *pb.GetTaskReq) (*pb.TaskResp, error) {
	r, err := l.reviewer(in.GetUserId(), claimRoles...)
	if err != nil {
		return nil, err
	}
	task, err := l.svcCtx.Store.GetTask(l.ctx, in.GetTaskId())
	if err != nil {
		return nil, l.mapStoreError(err, "get task")
	}
	if !canSeeTask(r, task) {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	view, err := l.taskView(task, true)
	if err != nil {
		return nil, l.mapStoreError(err, "load task evidence")
	}
	return &pb.TaskResp{Found: true, Task: view}, nil
}

// SubmitDecisionLogic 承载 SubmitDecision 接口的业务逻辑；每个请求新建一个实例。
type SubmitDecisionLogic struct{ base }

// NewSubmitDecisionLogic 绑定请求上下文与服务依赖。
func NewSubmitDecisionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitDecisionLogic {
	return &SubmitDecisionLogic{newBase(ctx, svcCtx)}
}

// SubmitDecision 提交人工结论（RVW-004、RVW-021）。拒绝至少带一个业务规范定义的政策码。
func (l *SubmitDecisionLogic) SubmitDecision(in *pb.SubmitDecisionReq) (*pb.SubmitDecisionResp, error) {
	r, err := l.reviewer(in.GetUserId(), claimRoles...)
	if err != nil {
		return nil, err
	}
	codes, err := validateVerdict(in.GetVerdict(), in.GetPolicyCodes())
	if err != nil {
		return nil, err
	}
	if len([]rune(in.GetNote())) > 500 || len(in.GetIdempotencyKey()) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	task, err := l.svcCtx.Store.GetTask(l.ctx, in.GetTaskId())
	if err != nil {
		return nil, l.mapStoreError(err, "load task for decision")
	}
	if !canSeeTask(r, task) {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	source := event.ReviewSourceHuman
	if task.Purpose == event.ReviewPurposeQA {
		source = event.ReviewSourceQA
	}
	seedText := ""
	if in.GetNominateSeed() && in.GetVerdict() == event.ReviewVerdictReject {
		content, err := l.svcCtx.Store.Snapshot(l.ctx, task.SnapshotHash)
		if err != nil {
			return nil, l.mapStoreError(err, "load snapshot for seed")
		}
		snap, err := snapshotOf(content)
		if err != nil {
			return nil, l.mapStoreError(err, "decode snapshot for seed")
		}
		seedText = cascade.JoinTexts(snap.Texts)
	}
	now := l.svcCtx.Now()
	decision, err := l.svcCtx.Store.SubmitHuman(l.ctx, r.UserID, task.ID, in.GetLeaseGeneration(), in.GetIdempotencyKey(),
		store.DecisionInput{
			Verdict: in.GetVerdict(), PolicyCodes: codes, PolicyVersion: l.svcCtx.Policy.Version, Source: source,
			Note: in.GetNote(), NominateSeed: in.GetNominateSeed(), SeedText: seedText,
		}, now)
	if err != nil {
		return nil, l.mapStoreError(err, "submit decision")
	}
	metrics.Decision(decision.Source, decision.Verdict, task.Purpose, task.SubmittedAtMs, now)
	l.observeReview(task, decision)
	return &pb.SubmitDecisionResp{
		DecisionId: decision.ID, Verdict: decision.Verdict, PolicyCodes: decision.PolicyCodes(),
		PolicyVersion: decision.PolicyVersion,
	}, nil
}

// observeReview 记录质检不一致与申诉改判（RVW-060）。
func (l *SubmitDecisionLogic) observeReview(task *store.Task, decision *store.Decision) {
	if task.SourceTaskID == 0 {
		return
	}
	original, err := l.svcCtx.Store.DecisionByTask(l.ctx, task.SourceTaskID)
	if err != nil || original == nil || original.Verdict == decision.Verdict {
		return
	}
	switch task.Purpose {
	case event.ReviewPurposeQA:
		metrics.QADisagreement(original.Source)
	case event.ReviewPurposeAppeal:
		metrics.AppealOverturn()
	}
}

// validateVerdict 校验结论与违规代码：通过不得带代码，拒绝至少一个代码；代码去重。
func validateVerdict(verdict string, rawCodes []string) ([]string, error) {
	var codes []string
	for _, code := range rawCodes {
		code = strings.TrimSpace(code)
		if !adpolicy.IsCode(code) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
	}
	switch verdict {
	case event.ReviewVerdictApprove:
		if len(codes) > 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	case event.ReviewVerdictReject:
		if len(codes) == 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	default:
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return codes, nil
}
