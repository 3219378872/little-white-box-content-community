package logic

import (
	"context"
	"errors"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"

	"esx/pkg/logging"
)

// base 是各广告 Logic 共用的请求上下文、依赖与日志。
type base struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// newBase 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func newBase(ctx context.Context, svcCtx *svc.ServiceContext) base {
	return base{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// requireUser 要求调用方已登录。
func requireUser(userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	return nil
}

// mapError 把存储与素材错误映射为业务错误码，未识别的错误记日志后返回系统错误。
func (b base) mapError(err error, action string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return errx.NewWithCode(errx.NotFound)
	case errors.Is(err, store.ErrRevisionConflict):
		return errx.NewWithCode(errx.ContentVersionConflict)
	case errors.Is(err, store.ErrAdvertiserExists):
		return errx.NewWithCode(errx.AdvertiserExists)
	case errors.Is(err, store.ErrAdvertiserNotApproved):
		return errx.NewWithCode(errx.AdvertiserRequired)
	case errors.Is(err, store.ErrQualificationMissing):
		return errx.NewWithCode(errx.AdQualificationRequired)
	case errors.Is(err, store.ErrAssetInvalid):
		return errx.NewWithCode(errx.AdMediaInvalid)
	case errors.Is(err, store.ErrMarketNotAllowed):
		return errx.NewWithCode(errx.AdIndustryUnsupported)
	case errors.Is(err, store.ErrAppealNotAllowed):
		return errx.NewWithCode(errx.AdAppealNotAllowed)
	case errors.Is(err, idempotencyx.ErrIdempotencyConflict):
		return errx.NewWithCode(errx.IdempotencyConflict)
	case errors.Is(err, assets.ErrTooLarge):
		return errx.NewWithCode(errx.FileTooLarge)
	case errors.Is(err, assets.ErrTypeNotAllowed):
		return errx.NewWithCode(errx.FileTypeNotAllowed)
	case errors.Is(err, assets.ErrEmpty):
		return errx.NewWithCode(errx.ParamError)
	default:
		// REL-022：业务日志不记录广告正文与证件内容，只记录动作与错误。
		b.Errorw(action+" failed", logging.Field("err", err.Error()))
		return errx.NewWithCode(errx.SystemError)
	}
}

// advertiserView 组装广告主及其资质的响应视图。
func advertiserView(full *store.AdvertiserWithQualifications) *pb.AdvertiserView {
	adv := full.Advertiser
	view := &pb.AdvertiserView{
		AdvertiserId: adv.ID, Name: adv.Name, Markets: adv.Markets(), Revision: adv.Revision,
		ApprovedRevision: adv.ApprovedRevision, ReviewStatus: adv.ReviewStatus, PolicyCodes: adv.PolicyCodes(),
		UpdatedAtMs: adv.UpdatedAtMs,
	}
	for _, q := range full.Qualifications {
		view.Qualifications = append(view.Qualifications, &pb.QualificationView{
			QualificationId: q.ID, Market: q.Market, Industry: q.Industry, DocumentMediaId: q.DocumentMediaID,
			ValidUntilMs: q.ValidUntilMs, Status: q.Status, SubmittedRevision: q.SubmittedRevision,
		})
	}
	return view
}

// adView 组装广告主视图。只有已发布到公开路径的过审素材才带公开地址（ADS-015）。
func (b base) adView(full *store.AdWithSnapshots, quals []store.Qualification) *pb.AdView {
	ad := full.Ad
	now := b.svcCtx.Now().UnixMilli()
	view := &pb.AdView{
		AdId: ad.ID, Revision: ad.Revision, ApprovedRevision: ad.ApprovedRevision, ReviewStatus: ad.ReviewStatus,
		ServingStatus: ad.ServingStatus, PolicyCodes: ad.PolicyCodes(), StartMs: ad.StartMs, EndMs: ad.EndMs,
		UpdatedAtMs: ad.UpdatedAtMs, PauseReason: ad.PauseReason, AppealedRevision: ad.AppealedRevision,
		Latest: b.content(full.Latest, false),
	}
	_, view.Appealable = store.AppealTarget(ad)
	if full.Approved != nil {
		view.Approved = b.content(full.Approved, ad.PublishedRevision >= ad.ApprovedRevision)
	}
	view.Eligible = eligible(ad, full.Approved, quals, now)
	return view
}

// content 组装快照内容；只有已发布到公开桶的版本才附带素材公开地址。
func (b base) content(snap *store.Snapshot, published bool) *pb.AdContent {
	if snap == nil {
		return nil
	}
	content := &pb.AdContent{
		Title: snap.Title, Body: snap.Body, Cta: snap.CTA, LandingUrl: snap.LandingURL, LandingDomain: snap.LandingDomain,
		Market: snap.Market, Language: snap.Language, Industry: snap.Industry, AdvertiserName: snap.AdvertiserName,
		Revision: snap.Revision,
	}
	for _, m := range snap.Media() {
		media := &pb.AdMedia{MediaId: m.MediaID, Sha256: m.SHA256}
		if published {
			media.PublicUrl = assets.PublicURLFor(b.svcCtx.Config.Assets.PublicBaseURL, m.SHA256)
		}
		content.Media = append(content.Media, media)
	}
	return content
}

// eligible 在读取时计算投放资格（ADS-010）：存在过审快照、投放状态为 serving、资质有效、在投放期内。
func eligible(ad *store.Ad, approved *store.Snapshot, quals []store.Qualification, nowMs int64) bool {
	if ad.ApprovedRevision == 0 || approved == nil || ad.ServingStatus != store.ServingServing {
		return false
	}
	if ad.StartMs > 0 && nowMs < ad.StartMs || ad.EndMs > 0 && nowMs >= ad.EndMs {
		return false
	}
	if adpolicy.DispositionOf(approved.Market, approved.Industry).NeedsQualification &&
		!store.ValidQualification(quals, approved.Market, approved.Industry, nowMs) {
		return false
	}
	return ad.PublishedRevision >= ad.ApprovedRevision
}
