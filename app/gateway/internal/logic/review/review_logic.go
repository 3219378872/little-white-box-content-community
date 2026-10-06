// Package review 是审核工作台的 Gateway 逻辑（SPEC-review-platform）。角色每次请求由 review-rpc
// 实时校验（RVW-050），Gateway 只传递已认证的用户标识。
package review

import (
	"context"

	"esx/app/ad/rpc/adservice"
	adslogic "esx/app/gateway/internal/logic/ads"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/review/rpc/reviewservice"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// base 是审核接口 Logic 的公共部分：带请求上下文的日志器与服务依赖。
type base struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// newBase 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func newBase(ctx context.Context, svcCtx *svc.ServiceContext) base {
	return base{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// userID 读取已认证审核员；审核接口都需要登录。
func (b base) userID() (int64, error) {
	return jwtx.GetUserIdFromContext(b.ctx)
}

// rpcError 记录审核 RPC 失败并映射为客户端可见的错误码。
func (b base) rpcError(err error, action string) error {
	b.Errorw(action+" RPC failed", logging.Field("err", err.Error()))
	return errx.FromRPCError(err)
}

// GetReviewerProfileLogic 承载 GetReviewerProfile 接口的业务逻辑；每个请求新建一个实例。
type GetReviewerProfileLogic struct{ base }

// NewGetReviewerProfileLogic 绑定请求上下文与服务依赖。
func NewGetReviewerProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewerProfileLogic {
	return &GetReviewerProfileLogic{newBase(ctx, svcCtx)}
}

// GetReviewerProfile 供客户端决定是否显示工作台入口（FX-112）；客户端守卫不替代服务端鉴权。
func (l *GetReviewerProfileLogic) GetReviewerProfile(*types.GetReviewerProfileReq) (*types.ReviewerProfileResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.GetReviewer(l.ctx, &reviewservice.GetReviewerReq{UserId: userID})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.GetReviewer")
	}
	return &types.ReviewerProfileResp{
		Active: resp.GetActive(), Roles: nonNil(resp.GetRoles()), Markets: nonNil(resp.GetMarkets()),
		Languages: nonNil(resp.GetLanguages()),
	}, nil
}

// GetReviewQueueLogic 承载 GetReviewQueue 接口的业务逻辑；每个请求新建一个实例。
type GetReviewQueueLogic struct{ base }

// NewGetReviewQueueLogic 绑定请求上下文与服务依赖。
func NewGetReviewQueueLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewQueueLogic {
	return &GetReviewQueueLogic{newBase(ctx, svcCtx)}
}

// GetReviewQueue 返回审核员有权处理的各用途队列积压与最老任务等待时长。
func (l *GetReviewQueueLogic) GetReviewQueue(*types.GetReviewQueueReq) (*types.ReviewQueueResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.GetQueueSummary(l.ctx, &reviewservice.GetQueueSummaryReq{UserId: userID})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.GetQueueSummary")
	}
	out := &types.ReviewQueueResp{Buckets: []types.ReviewQueueBucket{}, PolicyVersion: resp.GetPolicyVersion()}
	for _, b := range resp.GetBuckets() {
		out.Buckets = append(out.Buckets, types.ReviewQueueBucket{Purpose: b.GetPurpose(), Pending: b.GetPending(), OldestAgeMs: b.GetOldestAgeMs()})
	}
	return out, nil
}

// ClaimReviewTaskLogic 承载 ClaimReviewTask 接口的业务逻辑；每个请求新建一个实例。
type ClaimReviewTaskLogic struct{ base }

// NewClaimReviewTaskLogic 绑定请求上下文与服务依赖。
func NewClaimReviewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClaimReviewTaskLogic {
	return &ClaimReviewTaskLogic{newBase(ctx, svcCtx)}
}

// ClaimReviewTask 领取授权队列中的下一单（RVW-020、RVW-022）；队列为空返回 found=false。
func (l *ClaimReviewTaskLogic) ClaimReviewTask(req *types.ClaimReviewTaskReq) (*types.ReviewTaskResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.ClaimTask(l.ctx, &reviewservice.ClaimTaskReq{UserId: userID, Purpose: req.Purpose})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.ClaimTask")
	}
	return taskResp(resp), nil
}

// GetReviewTaskLogic 承载 GetReviewTask 接口的业务逻辑；每个请求新建一个实例。
type GetReviewTaskLogic struct{ base }

// NewGetReviewTaskLogic 绑定请求上下文与服务依赖。
func NewGetReviewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewTaskLogic {
	return &GetReviewTaskLogic{newBase(ctx, svcCtx)}
}

// GetReviewTask 读取审核员自己领取的任务详情，用于工作台刷新后恢复。
func (l *GetReviewTaskLogic) GetReviewTask(req *types.GetReviewTaskReq) (*types.ReviewTaskResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.GetTask(l.ctx, &reviewservice.GetTaskReq{UserId: userID, TaskId: req.TaskId})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.GetTask")
	}
	return taskResp(resp), nil
}

// RenewReviewTaskLogic 承载 RenewReviewTask 接口的业务逻辑；每个请求新建一个实例。
type RenewReviewTaskLogic struct{ base }

// NewRenewReviewTaskLogic 绑定请求上下文与服务依赖。
func NewRenewReviewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RenewReviewTaskLogic {
	return &RenewReviewTaskLogic{newBase(ctx, svcCtx)}
}

// RenewReviewTask 续期持有；持有失效或任务作废返回可区分错误（RVW-021）。
func (l *RenewReviewTaskLogic) RenewReviewTask(req *types.ReviewLeaseReq) (*types.ReviewTaskResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.RenewTask(l.ctx, &reviewservice.LeaseReq{
		UserId: userID, TaskId: req.TaskId, LeaseGeneration: req.LeaseGeneration,
	})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.RenewTask")
	}
	return taskResp(resp), nil
}

// ReleaseReviewTaskLogic 承载 ReleaseReviewTask 接口的业务逻辑；每个请求新建一个实例。
type ReleaseReviewTaskLogic struct{ base }

// NewReleaseReviewTaskLogic 绑定请求上下文与服务依赖。
func NewReleaseReviewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReleaseReviewTaskLogic {
	return &ReleaseReviewTaskLogic{newBase(ctx, svcCtx)}
}

// ReleaseReviewTask 主动归还租约，任务回到队列；LeaseGeneration 防止归还已被他人接手的租约。
func (l *ReleaseReviewTaskLogic) ReleaseReviewTask(req *types.ReviewLeaseReq) (*types.ReviewActionResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.ReviewService.ReleaseTask(l.ctx, &reviewservice.LeaseReq{
		UserId: userID, TaskId: req.TaskId, LeaseGeneration: req.LeaseGeneration,
	}); err != nil {
		return nil, l.rpcError(err, "ReviewService.ReleaseTask")
	}
	return &types.ReviewActionResp{Ok: true}, nil
}

// SubmitReviewDecisionLogic 承载 SubmitReviewDecision 接口的业务逻辑；每个请求新建一个实例。
type SubmitReviewDecisionLogic struct{ base }

// NewSubmitReviewDecisionLogic 绑定请求上下文与服务依赖。
func NewSubmitReviewDecisionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitReviewDecisionLogic {
	return &SubmitReviewDecisionLogic{newBase(ctx, svcCtx)}
}

// SubmitReviewDecision 提交结论；只有网络重试复用同一幂等键（FX-111、RVW-021）。
func (l *SubmitReviewDecisionLogic) SubmitReviewDecision(req *types.SubmitReviewDecisionReq) (*types.ReviewDecisionResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.SubmitDecision(l.ctx, &reviewservice.SubmitDecisionReq{
		UserId: userID, TaskId: req.TaskId, LeaseGeneration: req.LeaseGeneration, Verdict: req.Verdict,
		PolicyCodes: req.PolicyCodes, Note: req.Note, IdempotencyKey: req.IdempotencyKey, NominateSeed: req.NominateSeed,
	})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.SubmitDecision")
	}
	return &types.ReviewDecisionResp{
		DecisionId: resp.GetDecisionId(), Verdict: resp.GetVerdict(), PolicyCodes: nonNil(resp.GetPolicyCodes()),
		PolicyVersion: resp.GetPolicyVersion(),
	}, nil
}

// GetReviewEvidenceMediaLogic 承载 GetReviewEvidenceMedia 接口的业务逻辑；每个请求新建一个实例。
type GetReviewEvidenceMediaLogic struct{ base }

// NewGetReviewEvidenceMediaLogic 绑定请求上下文与服务依赖。
func NewGetReviewEvidenceMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewEvidenceMediaLogic {
	return &GetReviewEvidenceMediaLogic{newBase(ctx, svcCtx)}
}

// GetReviewEvidenceMedia 经审核平台授权后读取私有素材或证件（RVW-023、ADS-040）；未授权统一返回不存在。
func (l *GetReviewEvidenceMediaLogic) GetReviewEvidenceMedia(req *types.GetReviewEvidenceMediaReq) (*types.AdAssetContentResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	auth, err := l.svcCtx.ReviewService.AuthorizeEvidenceMedia(l.ctx, &reviewservice.AuthorizeEvidenceMediaReq{
		UserId: userID, TaskId: req.TaskId, MediaId: req.MediaId,
	})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.AuthorizeEvidenceMedia")
	}
	if !auth.GetAllowed() {
		return nil, errx.NewWithCode(errx.NotFound)
	}
	resp, err := l.svcCtx.AdService.ReadAsset(l.ctx, &adservice.ReadAssetReq{
		UserId: userID, AssetId: req.MediaId, ReviewerAuthorized: true,
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.ReadAsset")
	}
	return adslogic.AssetContent(resp), nil
}

// ListReviewSeedsLogic 承载 ListReviewSeeds 接口的业务逻辑；每个请求新建一个实例。
type ListReviewSeedsLogic struct{ base }

// NewListReviewSeedsLogic 绑定请求上下文与服务依赖。
func NewListReviewSeedsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListReviewSeedsLogic {
	return &ListReviewSeedsLogic{newBase(ctx, svcCtx)}
}

// ListReviewSeeds 按状态列出审核路由种子。
func (l *ListReviewSeedsLogic) ListReviewSeeds(req *types.ListReviewSeedsReq) (*types.ListReviewSeedsResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.ListSeeds(l.ctx, &reviewservice.ListSeedsReq{UserId: userID, Status: req.Status, Limit: req.Limit})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.ListSeeds")
	}
	out := &types.ListReviewSeedsResp{Seeds: []types.ReviewSeedItem{}}
	for _, seed := range resp.GetSeeds() {
		out.Seeds = append(out.Seeds, seedItem(seed))
	}
	return out, nil
}

// ConfirmReviewSeedLogic 承载 ConfirmReviewSeed 接口的业务逻辑；每个请求新建一个实例。
type ConfirmReviewSeedLogic struct{ base }

// NewConfirmReviewSeedLogic 绑定请求上下文与服务依赖。
func NewConfirmReviewSeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmReviewSeedLogic {
	return &ConfirmReviewSeedLogic{newBase(ctx, svcCtx)}
}

// ConfirmReviewSeed 需要政策管理权限且不同于提名人（RVW-030）。
func (l *ConfirmReviewSeedLogic) ConfirmReviewSeed(req *types.ReviewSeedActionReq) (*types.ReviewSeedResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.ConfirmSeed(l.ctx, &reviewservice.SeedActionReq{UserId: userID, SeedId: req.SeedId})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.ConfirmSeed")
	}
	return &types.ReviewSeedResp{Seed: seedItem(resp.GetSeed())}, nil
}

// RetireReviewSeedLogic 承载 RetireReviewSeed 接口的业务逻辑；每个请求新建一个实例。
type RetireReviewSeedLogic struct{ base }

// NewRetireReviewSeedLogic 绑定请求上下文与服务依赖。
func NewRetireReviewSeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RetireReviewSeedLogic {
	return &RetireReviewSeedLogic{newBase(ctx, svcCtx)}
}

// RetireReviewSeed 停用一条种子，使其不再参与路由检索；需要政策管理权限。
func (l *RetireReviewSeedLogic) RetireReviewSeed(req *types.ReviewSeedActionReq) (*types.ReviewSeedResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.ReviewService.RetireSeed(l.ctx, &reviewservice.SeedActionReq{UserId: userID, SeedId: req.SeedId})
	if err != nil {
		return nil, l.rpcError(err, "ReviewService.RetireSeed")
	}
	return &types.ReviewSeedResp{Seed: seedItem(resp.GetSeed())}, nil
}

// taskResp 把审核任务映射为 REST 结构；未领到任务时只返回 found=false。
func taskResp(resp *reviewservice.TaskResp) *types.ReviewTaskResp {
	out := &types.ReviewTaskResp{Found: resp.GetFound()}
	task := resp.GetTask()
	if !out.Found || task == nil {
		return out
	}
	item := &types.ReviewTaskItem{
		TaskId: task.GetTaskId(), BizType: task.GetBizType(), ObjectId: task.GetObjectId(),
		ObjectRevision: task.GetObjectRevision(), Purpose: task.GetPurpose(), Status: task.GetStatus(),
		Market: task.GetMarket(), Language: task.GetLanguage(), Industry: task.GetIndustry(), Priority: task.GetPriority(),
		DeadlineMs: task.GetDeadlineMs(), LeaseGeneration: task.GetLeaseGeneration(), LeaseUntilMs: task.GetLeaseUntilMs(),
		SnapshotJson: task.GetSnapshotJson(), Stages: []types.ReviewStageItem{},
		OriginalDecision: decisionItem(task.GetOriginalDecision()), Decision: decisionItem(task.GetDecision()),
		SubmittedAtMs: task.GetSubmittedAtMs(), EscalationReason: task.GetEscalationReason(),
		PolicyVersion: task.GetPolicyVersion(), Attempts: task.GetAttempts(),
	}
	for _, s := range task.GetStages() {
		item.Stages = append(item.Stages, types.ReviewStageItem{
			Stage: s.GetStage(), ComponentVersion: s.GetComponentVersion(), Shadow: s.GetShadow(), Outcome: s.GetOutcome(),
			Reason: s.GetReason(), OutputJson: s.GetOutputJson(), LatencyMs: s.GetLatencyMs(),
		})
	}
	out.Task = item
	return out
}

// decisionItem 映射一次审核决定；没有决定时返回 nil，前端据此区分“未决”。
func decisionItem(d *reviewservice.DecisionView) *types.ReviewDecisionItem {
	if d == nil {
		return nil
	}
	return &types.ReviewDecisionItem{
		DecisionId: d.GetDecisionId(), Verdict: d.GetVerdict(), PolicyCodes: nonNil(d.GetPolicyCodes()),
		PolicyVersion: d.GetPolicyVersion(), Source: d.GetSource(), DecidedAtMs: d.GetDecidedAtMs(),
	}
}

// seedItem 把路由种子映射为 REST 结构；nil 返回零值。
func seedItem(s *reviewservice.SeedView) types.ReviewSeedItem {
	if s == nil {
		return types.ReviewSeedItem{}
	}
	return types.ReviewSeedItem{
		SeedId: s.GetSeedId(), IssueCode: s.GetIssueCode(), Market: s.GetMarket(), Language: s.GetLanguage(),
		Text: s.GetText(), Status: s.GetStatus(), NominatedBy: s.GetNominatedBy(), ConfirmedBy: s.GetConfirmedBy(),
		SourceTaskId: s.GetSourceTaskId(), UpdatedAtMs: s.GetUpdatedAtMs(),
	}
}

// nonNil 把 nil 切片规范为空切片，使 JSON 输出 [] 而不是 null。
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
