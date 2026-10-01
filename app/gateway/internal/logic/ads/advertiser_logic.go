package ads

import (
	"context"

	"esx/app/ad/rpc/adservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	logx "esx/pkg/logging"
)

type base struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func newBase(ctx context.Context, svcCtx *svc.ServiceContext) base {
	return base{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// userID 读取已认证用户；广告主接口都需要认证（ADS-040）。
func (b base) userID() (int64, error) {
	userID, err := jwtx.GetUserIdFromContext(b.ctx)
	if err != nil || userID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	return userID, nil
}

func (b base) rpcError(err error, action string) error {
	b.Errorw(action+" RPC failed", logx.Field("err", err.Error()))
	return errx.FromRPCError(err)
}

type GetMyAdvertiserLogic struct{ base }

func NewGetMyAdvertiserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMyAdvertiserLogic {
	return &GetMyAdvertiserLogic{newBase(ctx, svcCtx)}
}

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

type ApplyAdvertiserLogic struct{ base }

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

type AddAdQualificationLogic struct{ base }

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

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
