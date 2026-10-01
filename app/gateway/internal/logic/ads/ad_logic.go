package ads

import (
	"context"
	"strconv"
	"strings"

	"esx/app/ad/rpc/adservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"
)

type ListAdsLogic struct{ base }

func NewListAdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAdsLogic {
	return &ListAdsLogic{newBase(ctx, svcCtx)}
}

func (l *ListAdsLogic) ListAds(req *types.ListAdsReq) (*types.ListAdsResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	var cursor int64
	if strings.TrimSpace(req.Cursor) != "" {
		cursor, err = strconv.ParseInt(req.Cursor, 10, 64)
		if err != nil || cursor < 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
	}
	if req.PageSize < 0 || req.PageSize > 50 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	resp, err := l.svcCtx.AdService.ListAds(l.ctx, &adservice.ListAdsReq{UserId: userID, Cursor: cursor, PageSize: req.PageSize})
	if err != nil {
		return nil, l.rpcError(err, "AdService.ListAds")
	}
	out := &types.ListAdsResp{Ads: []types.AdItem{}, HasMore: resp.GetHasMore()}
	if resp.GetNextCursor() > 0 {
		out.NextCursor = strconv.FormatInt(resp.GetNextCursor(), 10)
	}
	for _, ad := range resp.GetAds() {
		out.Ads = append(out.Ads, adItem(ad))
	}
	return out, nil
}

type CreateAdLogic struct{ base }

func NewCreateAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAdLogic {
	return &CreateAdLogic{newBase(ctx, svcCtx)}
}

// CreateAd 创建广告并自动送审（ADS-011）。
func (l *CreateAdLogic) CreateAd(req *types.CreateAdReq) (*types.AdResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.CreateAd(l.ctx, &adservice.CreateAdReq{
		UserId: userID, IdempotencyKey: req.IdempotencyKey,
		Input: &adservice.AdInput{
			Title: req.Title, Body: req.Body, Cta: req.Cta, LandingUrl: req.LandingUrl, MediaIds: req.MediaIds,
			Market: req.Market, Industry: req.Industry, StartMs: req.StartMs, EndMs: req.EndMs,
		},
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.CreateAd")
	}
	return &types.AdResp{Ad: adItem(resp.GetAd())}, nil
}

type GetAdLogic struct{ base }

func NewGetAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAdLogic {
	return &GetAdLogic{newBase(ctx, svcCtx)}
}

func (l *GetAdLogic) GetAd(req *types.GetAdReq) (*types.AdResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.GetAd(l.ctx, &adservice.GetAdReq{UserId: userID, AdId: req.AdId})
	if err != nil {
		return nil, l.rpcError(err, "AdService.GetAd")
	}
	return &types.AdResp{Ad: adItem(resp.GetAd())}, nil
}

type UpdateAdLogic struct{ base }

func NewUpdateAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAdLogic {
	return &UpdateAdLogic{newBase(ctx, svcCtx)}
}

// UpdateAd 编辑产生新 revision；审核期间继续投放上一过审版本（ADS-011、ADS-013）。
func (l *UpdateAdLogic) UpdateAd(req *types.UpdateAdReq) (*types.AdResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.UpdateAd(l.ctx, &adservice.UpdateAdReq{
		UserId: userID, AdId: req.AdId, ExpectedRevision: req.ExpectedRevision, IdempotencyKey: req.IdempotencyKey,
		Input: &adservice.AdInput{
			Title: req.Title, Body: req.Body, Cta: req.Cta, LandingUrl: req.LandingUrl, MediaIds: req.MediaIds,
			Market: req.Market, Industry: req.Industry, StartMs: req.StartMs, EndMs: req.EndMs,
		},
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.UpdateAd")
	}
	return &types.AdResp{Ad: adItem(resp.GetAd())}, nil
}

type HideAdLogic struct{ base }

func NewHideAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HideAdLogic {
	return &HideAdLogic{newBase(ctx, svcCtx)}
}

// HideAd 隐藏广告（ADS-026）：已认证用户按用户；匿名用户按会话，不使用 anonymousId。
func (l *HideAdLogic) HideAd(req *types.HideAdReq) (*types.AdActionResp, error) {
	userID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	sessionID := strings.TrimSpace(req.SessionId)
	if req.AdId <= 0 || (userID <= 0 && sessionID == "") || len(sessionID) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if userID < 0 {
		userID = 0
	}
	if _, err := l.svcCtx.AdService.HideAd(l.ctx, &adservice.HideAdReq{UserId: userID, SessionId: sessionID, AdId: req.AdId}); err != nil {
		return nil, l.rpcError(err, "AdService.HideAd")
	}
	return &types.AdActionResp{Ok: true}, nil
}

type ReportAdLogic struct{ base }

func NewReportAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReportAdLogic {
	return &ReportAdLogic{newBase(ctx, svcCtx)}
}

// ReportAd 举报广告（ADS-030）：生成投后复审任务并对举报人隐藏（ADS-026）；匿名用户按会话识别。
func (l *ReportAdLogic) ReportAd(req *types.ReportAdReq) (*types.ReportAdResp, error) {
	userID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	sessionID := strings.TrimSpace(req.SessionId)
	if req.AdId <= 0 || (userID <= 0 && sessionID == "") || len(sessionID) > 128 || len(req.Reason) > 32 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if userID < 0 {
		userID = 0
	}
	resp, err := l.svcCtx.AdService.ReportAd(l.ctx, &adservice.ReportAdReq{
		UserId: userID, SessionId: sessionID, AdId: req.AdId, Reason: strings.TrimSpace(req.Reason),
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.ReportAd")
	}
	return &types.ReportAdResp{Counted: resp.GetCounted()}, nil
}

type AppealAdLogic struct{ base }

func NewAppealAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AppealAdLogic {
	return &AppealAdLogic{newBase(ctx, svcCtx)}
}

// AppealAd 对被拒或被下线的 revision 申诉，每个 revision 一次（ADS-014）。
func (l *AppealAdLogic) AppealAd(req *types.AppealAdReq) (*types.AdResp, error) {
	userID, err := l.userID()
	if err != nil {
		return nil, err
	}
	resp, err := l.svcCtx.AdService.AppealAd(l.ctx, &adservice.AppealAdReq{
		UserId: userID, AdId: req.AdId, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, l.rpcError(err, "AdService.AppealAd")
	}
	return &types.AdResp{Ad: adItem(resp.GetAd())}, nil
}

func adItem(ad *adservice.AdView) types.AdItem {
	if ad == nil {
		return types.AdItem{PolicyCodes: []string{}}
	}
	item := types.AdItem{
		AdId: ad.GetAdId(), Revision: ad.GetRevision(), ApprovedRevision: ad.GetApprovedRevision(),
		ReviewStatus: ad.GetReviewStatus(), ServingStatus: ad.GetServingStatus(), PolicyCodes: nonNil(ad.GetPolicyCodes()),
		Latest: adContent(ad.GetLatest()), StartMs: ad.GetStartMs(), EndMs: ad.GetEndMs(), Eligible: ad.GetEligible(),
		UpdatedAtMs: ad.GetUpdatedAtMs(), PauseReason: ad.GetPauseReason(),
		Appealable: ad.GetAppealable(), AppealedRevision: ad.GetAppealedRevision(),
	}
	if ad.GetApproved() != nil {
		approved := adContent(ad.GetApproved())
		item.Approved = &approved
	}
	return item
}

func adContent(content *adservice.AdContent) types.AdContentItem {
	out := types.AdContentItem{Media: []types.AdMediaItem{}}
	if content == nil {
		return out
	}
	out.Title, out.Body, out.Cta = content.GetTitle(), content.GetBody(), content.GetCta()
	out.LandingUrl, out.LandingDomain = content.GetLandingUrl(), content.GetLandingDomain()
	out.Market, out.Language, out.Industry = content.GetMarket(), content.GetLanguage(), content.GetIndustry()
	out.AdvertiserName, out.Revision = content.GetAdvertiserName(), content.GetRevision()
	for _, m := range content.GetMedia() {
		out.Media = append(out.Media, types.AdMediaItem{MediaId: m.GetMediaId(), Sha256: m.GetSha256(), PublicUrl: m.GetPublicUrl()})
	}
	return out
}
