package logic

import (
	"context"
	"strings"

	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
)

// ReportAdLogic 承载 ReportAd 接口的业务逻辑；每个请求新建一个实例。
type ReportAdLogic struct{ base }

// NewReportAdLogic 绑定请求上下文与服务依赖。
func NewReportAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReportAdLogic {
	return &ReportAdLogic{newBase(ctx, svcCtx)}
}

// ReportAd 记录举报并生成投后复审任务（ADS-030），同时对举报人隐藏该广告（ADS-026）：已认证用户
// 30 天，匿名用户只在当前会话内。身份与隐藏、频控一致，不使用 anonymousId。
func (l *ReportAdLogic) ReportAd(in *pb.ReportAdReq) (*pb.ReportAdResp, error) {
	identity, err := serving.Identity(in.GetUserId(), strings.TrimSpace(in.GetSessionId()))
	reason := strings.TrimSpace(in.GetReason())
	if err != nil || in.GetAdId() <= 0 || !store.IsReportReason(reason) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	now := l.svcCtx.Now()
	result, err := l.svcCtx.Store.ReportAd(l.ctx, store.ReportInput{AdID: in.GetAdId(), ReporterKey: identity, Reason: reason}, now)
	if err != nil {
		return nil, l.mapError(err, "report ad")
	}
	outcome := "duplicate"
	if result.Counted {
		outcome = "counted"
	}
	metrics.Report(outcome)
	if err := serving.Hide(l.ctx, l.svcCtx.Redis, identity, in.GetAdId(), now); err != nil {
		// 举报已持久化并送审；隐藏失败只影响本人后续是否还会看到，返回错误让客户端重试（举报去重）。
		return nil, l.mapError(err, "hide reported ad")
	}
	return &pb.ReportAdResp{Counted: result.Counted}, nil
}

// AppealAdLogic 承载 AppealAd 接口的业务逻辑；每个请求新建一个实例。
type AppealAdLogic struct{ base }

// NewAppealAdLogic 绑定请求上下文与服务依赖。
func NewAppealAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AppealAdLogic {
	return &AppealAdLogic{newBase(ctx, svcCtx)}
}

// AppealAd 对被拒或被下线的 revision 申诉（ADS-014）；不可申诉时返回 7106，越权返回不存在。
func (l *AppealAdLogic) AppealAd(in *pb.AppealAdReq) (*pb.AdResp, error) {
	if err := requireUser(in.GetUserId()); err != nil {
		return nil, err
	}
	if in.GetAdId() <= 0 || len(in.GetIdempotencyKey()) > 128 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	full, err := l.svcCtx.Store.AppealAd(l.ctx, in.GetUserId(), in.GetAdId(), in.GetIdempotencyKey(), l.svcCtx.Now())
	if err != nil {
		return nil, l.mapError(err, "appeal ad")
	}
	metrics.Appeal()
	return l.response(full)
}
