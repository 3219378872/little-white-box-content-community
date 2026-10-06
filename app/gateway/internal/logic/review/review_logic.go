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

type base struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func newBase(ctx context.Context, svcCtx *svc.ServiceContext) base {
	return base{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// userID 读取已认证审核员；审核接口都需要登录。
func (b base) userID() (int64, error) {
	return jwtx.GetUserIdFromContext(b.ctx)
}

func (b base) rpcError(err error, action string) error {
	b.Errorw(action+" RPC failed", logging.Field("err", err.Error()))
	return errx.FromRPCError(err)
}

type GetReviewerProfileLogic struct{ base }

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

type GetReviewQueueLogic struct{ base }

func NewGetReviewQueueLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewQueueLogic {
	return &GetReviewQueueLogic{newBase(ctx, svcCtx)}
}

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

type ClaimReviewTaskLogic struct{ base }

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

type GetReviewTaskLogic struct{ base }

func NewGetReviewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetReviewTaskLogic {
	return &GetReviewTaskLogic{newBase(ctx, svcCtx)}
}

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

type RenewReviewTaskLogic struct{ base }

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

type ReleaseReviewTaskLogic struct{ base }

func NewReleaseReviewTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReleaseReviewTaskLogic {
	return &ReleaseReviewTaskLogic{newBase(ctx, svcCtx)}
}

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

type SubmitReviewDecisionLogic struct{ base }

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

type GetReviewEvidenceMediaLogic struct{ base }

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

type ListReviewSeedsLogic struct{ base }

func NewListReviewSeedsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListReviewSeedsLogic {
	return &ListReviewSeedsLogic{newBase(ctx, svcCtx)}
}

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

type ConfirmReviewSeedLogic struct{ base }

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

type RetireReviewSeedLogic struct{ base }

func NewRetireReviewSeedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RetireReviewSeedLogic {
	return &RetireReviewSeedLogic{newBase(ctx, svcCtx)}
}

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

func decisionItem(d *reviewservice.DecisionView) *types.ReviewDecisionItem {
	if d == nil {
		return nil
	}
	return &types.ReviewDecisionItem{
		DecisionId: d.GetDecisionId(), Verdict: d.GetVerdict(), PolicyCodes: nonNil(d.GetPolicyCodes()),
		PolicyVersion: d.GetPolicyVersion(), Source: d.GetSource(), DecidedAtMs: d.GetDecidedAtMs(),
	}
}

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

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
