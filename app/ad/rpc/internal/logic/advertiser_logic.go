package logic

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
)

// ApplyAdvertiserLogic 承载 ApplyAdvertiser 接口的业务逻辑；每个请求新建一个实例。
type ApplyAdvertiserLogic struct{ base }

// NewApplyAdvertiserLogic 绑定请求上下文与服务依赖。
func NewApplyAdvertiserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ApplyAdvertiserLogic {
	return &ApplyAdvertiserLogic{newBase(ctx, svcCtx)}
}

// ApplyAdvertiser 申请或修改广告主（ADS-001）；每个用户最多一个广告主。
func (l *ApplyAdvertiserLogic) ApplyAdvertiser(in *pb.ApplyAdvertiserReq) (*pb.AdvertiserResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.GetName())
	markets, err := validMarkets(in.GetMarkets())
	if err != nil || utf8.RuneCountInString(name) < 2 || utf8.RuneCountInString(name) > 64 ||
		in.GetExpectedRevision() < 0 || len(in.GetIdempotencyKey()) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	full, err := l.svcCtx.Store.ApplyAdvertiser(l.ctx, store.AdvertiserInput{
		UserID: in.GetUserId(), Name: name, Markets: markets, ExpectedRevision: in.GetExpectedRevision(),
		IdempotencyKey: in.GetIdempotencyKey(),
	}, l.svcCtx.Now())
	if err != nil {
		return nil, l.mapError(err, "apply advertiser")
	}
	return &pb.AdvertiserResp{Found: true, Advertiser: advertiserView(full)}, nil
}

// GetMyAdvertiserLogic 承载 GetMyAdvertiser 接口的业务逻辑；每个请求新建一个实例。
type GetMyAdvertiserLogic struct{ base }

// NewGetMyAdvertiserLogic 绑定请求上下文与服务依赖。
func NewGetMyAdvertiserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMyAdvertiserLogic {
	return &GetMyAdvertiserLogic{newBase(ctx, svcCtx)}
}

// GetMyAdvertiser 返回本人的广告主；尚未申请时 Found=false 而不是报错。
func (l *GetMyAdvertiserLogic) GetMyAdvertiser(in *pb.GetMyAdvertiserReq) (*pb.AdvertiserResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	full, err := l.svcCtx.Store.GetAdvertiserByUser(l.ctx, in.GetUserId())
	if errors.Is(err, store.ErrNotFound) {
		return &pb.AdvertiserResp{Found: false}, nil
	}
	if err != nil {
		return nil, l.mapError(err, "get advertiser")
	}
	return &pb.AdvertiserResp{Found: true, Advertiser: advertiserView(full)}, nil
}

// AddQualificationLogic 承载 AddQualification 接口的业务逻辑；每个请求新建一个实例。
type AddQualificationLogic struct{ base }

// NewAddQualificationLogic 绑定请求上下文与服务依赖。
func NewAddQualificationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddQualificationLogic {
	return &AddQualificationLogic{newBase(ctx, svcCtx)}
}

// AddQualification 提交行业资质；证件必须是本人上传的私有 document 资产（ADS-040）。
func (l *AddQualificationLogic) AddQualification(in *pb.AddQualificationReq) (*pb.AdvertiserResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	market, industry := strings.ToUpper(strings.TrimSpace(in.GetMarket())), strings.ToUpper(strings.TrimSpace(in.GetIndustry()))
	if !adpolicy.IsMarket(market) || !adpolicy.IsIndustry(industry) || in.GetDocumentMediaId() <= 0 ||
		in.GetValidUntilMs() <= l.svcCtx.Now().UnixMilli() || in.GetExpectedRevision() <= 0 || len(in.GetIdempotencyKey()) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	full, err := l.svcCtx.Store.AddQualification(l.ctx, store.QualificationInput{
		UserID: in.GetUserId(), Market: market, Industry: industry, DocumentAssetID: in.GetDocumentMediaId(),
		ValidUntilMs: in.GetValidUntilMs(), ExpectedRevision: in.GetExpectedRevision(), IdempotencyKey: in.GetIdempotencyKey(),
	}, l.svcCtx.Now())
	if err != nil {
		return nil, l.mapError(err, "add qualification")
	}
	return &pb.AdvertiserResp{Found: true, Advertiser: advertiserView(full)}, nil
}

// validMarkets 规范化并去重市场代码，至少需要一个有效市场。
func validMarkets(raw []string) ([]string, error) {
	var markets []string
	for _, m := range raw {
		m = strings.ToUpper(strings.TrimSpace(m))
		if !adpolicy.IsMarket(m) {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if !slices.Contains(markets, m) {
			markets = append(markets, m)
		}
	}
	if len(markets) == 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	return markets, nil
}
