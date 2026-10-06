package ads

import (
	"context"

	"esx/app/ad/rpc/adservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// base 是广告主接口 Logic 的公共部分：带请求上下文的日志器与服务依赖。
type base struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// newBase 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func newBase(ctx context.Context, svcCtx *svc.ServiceContext) base {
	return base{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// userID 读取已认证用户；广告主接口都需要认证（ADS-040）。
func (b base) userID() (int64, error) {
	return jwtx.GetUserIdFromContext(b.ctx)
}

// rpcError 记录广告 RPC 失败并映射为客户端可见的错误码。
func (b base) rpcError(err error, action string) error {
	b.Errorw(action+" RPC failed", logging.Field("err", err.Error()))
	return errx.FromRPCError(err)
}

// GetMyAdvertiserLogic 承载 GetMyAdvertiser 接口的业务逻辑；每个请求新建一个实例。
type GetMyAdvertiserLogic struct{ base }

// NewGetMyAdvertiserLogic 绑定请求上下文与服务依赖。
func NewGetMyAdvertiserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMyAdvertiserLogic {
	return &GetMyAdvertiserLogic{newBase(ctx, svcCtx)}
}

// GetMyAdvertiser 返回当前用户的广告主资料；未申请时 found=false。
func (l *GetMyAdvertiserLogic) GetMyAdvertiser(*types.GetMyAdvertiserReq) (*types.AdvertiserResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.GetMyAdvertiser(l.ctx, &adservice.GetMyAdvertiserReq{UserId: userID})
	if err != nil {
		return nil, l.rpcError(err, "AdService.GetMyAdvertiser")
	}
	return advertiserResp(resp), nil
}

// ApplyAdvertiserLogic 承载 ApplyAdvertiser 接口的业务逻辑；每个请求新建一个实例。
type ApplyAdvertiserLogic struct{ base }

// NewApplyAdvertiserLogic 绑定请求上下文与服务依赖。
func NewApplyAdvertiserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ApplyAdvertiserLogic {
	return &ApplyAdvertiserLogic{newBase(ctx, svcCtx)}
}

// ApplyAdvertiser 申请或修改广告主（ADS-001）；修改带 expectedRevision 与幂等键（ADS-013）。
func (l *ApplyAdvertiserLogic) ApplyAdvertiser(req *types.ApplyAdvertiserReq) (*types.AdvertiserResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.ApplyAdvertiser(l.ctx, &adservice.ApplyAdvertiserReq{
		UserId: userID, Name: req.Name, Markets: req.Markets, ExpectedRevision: req.ExpectedRevision,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.ApplyAdvertiser")
	}
	return advertiserResp(resp), nil
}

// AddAdQualificationLogic 承载 AddAdQualification 接口的业务逻辑；每个请求新建一个实例。
type AddAdQualificationLogic struct{ base }

// NewAddAdQualificationLogic 绑定请求上下文与服务依赖。
func NewAddAdQualificationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddAdQualificationLogic {
	return &AddAdQualificationLogic{newBase(ctx, svcCtx)}
}

// AddAdQualification 提交行业资质，随广告主新 revision 送审（ADS-001、ADS-002）。
func (l *AddAdQualificationLogic) AddAdQualification(req *types.AddQualificationReq) (*types.AdvertiserResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.AddQualification(l.ctx, &adservice.AddQualificationReq{
		UserId: userID, Market: req.Market, Industry: req.Industry, DocumentMediaId: req.DocumentAssetId,
		ValidUntilMs: req.ValidUntilMs, ExpectedRevision: req.ExpectedRevision, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.AddQualification")
	}
	return advertiserResp(resp), nil
}

// advertiserResp 把广告主资料及其资质映射为 REST 结构；未申请时只返回 found=false。
func advertiserResp(resp *adservice.AdvertiserResp) *types.AdvertiserResp {
	out := &types.AdvertiserResp{Found: resp.GetFound()}
	adv := resp.GetAdvertiser()
	if !out.Found || adv == nil {
		return out
	}
	item := &types.AdvertiserItem{
		AdvertiserId: adv.GetAdvertiserId(), Name: adv.GetName(), Markets: nonNil(adv.GetMarkets()),
		Revision: adv.GetRevision(), ApprovedRevision: adv.GetApprovedRevision(), ReviewStatus: adv.GetReviewStatus(),
		PolicyCodes: nonNil(adv.GetPolicyCodes()), Qualifications: []types.AdQualificationItem{},
		UpdatedAtMs: adv.GetUpdatedAtMs(),
	}
	for _, q := range adv.GetQualifications() {
		item.Qualifications = append(item.Qualifications, types.AdQualificationItem{
			QualificationId: q.GetQualificationId(), Market: q.GetMarket(), Industry: q.GetIndustry(),
			DocumentAssetId: q.GetDocumentMediaId(), ValidUntilMs: q.GetValidUntilMs(), Status: q.GetStatus(),
			SubmittedRevision: q.GetSubmittedRevision(),
		})
	}
	out.Advertiser = item
	return out
}

// nonNil 把 nil 切片规范为空切片，使 JSON 输出 [] 而不是 null。
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
