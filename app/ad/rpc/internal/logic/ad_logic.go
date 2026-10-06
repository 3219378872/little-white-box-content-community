package logic

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
)

// MaxAdMedia 是单条广告的素材上限。
const MaxAdMedia = 3

// CreateAdLogic 承载 CreateAd 接口的业务逻辑；每个请求新建一个实例。
type CreateAdLogic struct{ base }

// NewCreateAdLogic 绑定请求上下文与服务依赖。
func NewCreateAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAdLogic {
	return &CreateAdLogic{newBase(ctx, svcCtx)}
}

// CreateAd 创建广告并自动送审（ADS-011）；落地页结构不合规直接以 LANDING.URL 拒绝（ADS-016）。
func (l *CreateAdLogic) CreateAd(in *pb.CreateAdReq) (*pb.AdResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	input, err := validateAdInput(in.GetInput(), in.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	full, err := l.svcCtx.Store.CreateAd(l.ctx, in.GetUserId(), input, in.GetIdempotencyKey(), l.svcCtx.Now())
	if err != nil {
		return nil, l.mapError(err, "create ad")
	}
	return l.response(full)
}

// UpdateAdLogic 承载 UpdateAd 接口的业务逻辑；每个请求新建一个实例。
type UpdateAdLogic struct{ base }

// NewUpdateAdLogic 绑定请求上下文与服务依赖。
func NewUpdateAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateAdLogic {
	return &UpdateAdLogic{newBase(ctx, svcCtx)}
}

// UpdateAd 编辑产生新 revision；审核期间继续投放上一过审快照（ADS-011、ADS-013）。
func (l *UpdateAdLogic) UpdateAd(in *pb.UpdateAdReq) (*pb.AdResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	if in.GetAdId() <= 0 || in.GetExpectedRevision() <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	input, err := validateAdInput(in.GetInput(), in.GetIdempotencyKey())
	if err != nil {
		return nil, err
	}
	full, err := l.svcCtx.Store.UpdateAd(l.ctx, in.GetUserId(), in.GetAdId(), in.GetExpectedRevision(), input,
		in.GetIdempotencyKey(), l.svcCtx.Now())
	if err != nil {
		return nil, l.mapError(err, "update ad")
	}
	return l.response(full)
}

// GetAdLogic 承载 GetAd 接口的业务逻辑；每个请求新建一个实例。
type GetAdLogic struct{ base }

// NewGetAdLogic 绑定请求上下文与服务依赖。
func NewGetAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAdLogic {
	return &GetAdLogic{newBase(ctx, svcCtx)}
}

// GetAd 返回本人的一条广告；不存在与越权统一返回不存在。
func (l *GetAdLogic) GetAd(in *pb.GetAdReq) (*pb.AdResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	full, err := l.svcCtx.Store.GetAd(l.ctx, in.GetUserId(), in.GetAdId())
	if err != nil {
		return nil, l.mapError(err, "get ad")
	}
	return l.response(full)
}

// ListAdsLogic 承载 ListAds 接口的业务逻辑；每个请求新建一个实例。
type ListAdsLogic struct{ base }

// NewListAdsLogic 绑定请求上下文与服务依赖。
func NewListAdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAdsLogic {
	return &ListAdsLogic{newBase(ctx, svcCtx)}
}

// ListAds 分页列出本人的广告，并按广告主缓存资质，避免同页重复查询。
func (l *ListAdsLogic) ListAds(in *pb.ListAdsReq) (*pb.ListAdsResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	ads, next, hasMore, err := l.svcCtx.Store.ListAds(l.ctx, in.GetUserId(), in.GetCursor(), int(in.GetPageSize()))
	if err != nil {
		return nil, l.mapError(err, "list ads")
	}
	resp := &pb.ListAdsResp{NextCursor: next, HasMore: hasMore}
	quals := map[int64][]store.Qualification{}
	for i := range ads {
		advertiserID := ads[i].Ad.AdvertiserID
		if _, ok := quals[advertiserID]; !ok {
			q, err := l.svcCtx.Store.QualificationsOf(l.ctx, advertiserID)
			if err != nil {
				return nil, l.mapError(err, "list qualifications")
			}
			quals[advertiserID] = q
		}
		resp.Ads = append(resp.Ads, l.adView(&ads[i], quals[advertiserID]))
	}
	return resp, nil
}

// response 载入广告主资质后组装广告响应，资质用于计算投放资格。
func (b base) response(full *store.AdWithSnapshots) (*pb.AdResp, error) {
	quals, err := b.svcCtx.Store.QualificationsOf(b.ctx, full.Ad.AdvertiserID)
	if err != nil {
		return nil, b.mapError(err, "load qualifications")
	}
	return &pb.AdResp{Ad: b.adView(full, quals)}, nil
}

// validateAdInput 规范化并校验广告输入：文案长度、市场与行业、落地页结构、投放期与素材数量；素材 ID 去重。
func validateAdInput(in *pb.AdInput, idempotencyKey string) (store.AdInput, error) {
	invalid := errx.NewWithCode(errx.ParamError)
	if in == nil || len(idempotencyKey) > 128 {
		return store.AdInput{}, invalid
	}
	title, body, cta := strings.TrimSpace(in.GetTitle()), strings.TrimSpace(in.GetBody()), strings.TrimSpace(in.GetCta())
	if !runeLen(title, 1, 100) || !runeLen(body, 1, 500) || !runeLen(cta, 1, 32) {
		return store.AdInput{}, invalid
	}
	market, industry := strings.ToUpper(strings.TrimSpace(in.GetMarket())), strings.ToUpper(strings.TrimSpace(in.GetIndustry()))
	if !adpolicy.IsMarket(market) || !adpolicy.IsIndustry(industry) {
		return store.AdInput{}, errx.NewWithCode(errx.AdIndustryUnsupported)
	}
	domain, ok := adpolicy.LandingURLValid(in.GetLandingUrl())
	if !ok {
		return store.AdInput{}, errx.NewWithCode(errx.AdLandingInvalid)
	}
	if in.GetStartMs() < 0 || in.GetEndMs() < 0 || (in.GetEndMs() > 0 && in.GetEndMs() <= in.GetStartMs()) {
		return store.AdInput{}, invalid
	}
	var media []int64
	for _, id := range in.GetMediaIds() {
		if id <= 0 {
			return store.AdInput{}, invalid
		}
		if !slices.Contains(media, id) {
			media = append(media, id)
		}
	}
	if len(media) > MaxAdMedia {
		return store.AdInput{}, invalid
	}
	return store.AdInput{
		Title: title, Body: body, CTA: cta, LandingURL: in.GetLandingUrl(), LandingDomain: domain, MediaIDs: media,
		Market: market, Industry: industry, StartMs: in.GetStartMs(), EndMs: in.GetEndMs(),
	}, nil
}

// runeLen 按字符数（而非字节数）检查长度范围。
func runeLen(value string, minLen, maxLen int) bool {
	n := utf8.RuneCountInString(value)
	return n >= minLen && n <= maxLen
}
