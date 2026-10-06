package feed

import (
	"context"
	"strings"
	"time"

	"esx/app/ad/rpc/adservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/adpolicy"

	"esx/pkg/logging"
	metric "esx/pkg/metrics"
)

const sponsoredDisclosure = "sponsored"

var sponsoredRequests = metric.NewCounterVec(&metric.CounterVecOpts{
	Namespace: "esx", Subsystem: "gateway", Name: "sponsored_slot_requests_total",
	Help: "Recommend feed sponsored slot lookups by outcome", Labels: []string{"outcome"},
})

// sponsoredRequest 是一页推荐流的广告槽位请求。
type sponsoredRequest struct {
	userID    int64
	sessionID string
	requestID string
	cursor    string
	scene     string
	market    string
	pageSize  int32
}

type sponsoredResult struct {
	slots []*adservice.SponsoredSlot
}

// startSponsored 与帖子 RPC 并行查询广告槽位（DES-sponsored-ads「组装」）。广告服务失败、超时或
// 无法确认资格时只返回空结果，绝不影响帖子页面（ADS-022）。
func startSponsored(ctx context.Context, svcCtx *svc.ServiceContext, req sponsoredRequest) <-chan sponsoredResult {
	out := make(chan sponsoredResult, 1)
	if svcCtx.AdService == nil || (req.market != "" && !adpolicy.IsMarket(req.market)) {
		sponsoredRequests.Inc("skipped")
		out <- sponsoredResult{}
		return out
	}
	timeout := time.Duration(max(svcCtx.Config.SponsoredTimeoutMs, 1)) * time.Millisecond
	go func() {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		resp, err := svcCtx.AdService.GetSponsoredSlots(callCtx, &adservice.GetSponsoredSlotsReq{
			UserId: req.userID, SessionId: req.sessionID, RequestId: req.requestID, Cursor: req.cursor,
			Scene: req.scene, Market: req.market, PageItems: req.pageSize,
		})
		if err != nil {
			outcome := "error"
			if callCtx.Err() != nil {
				outcome = "timeout"
			}
			sponsoredRequests.Inc(outcome)
			logging.WithContext(ctx).Infow("sponsored slots degraded", logging.Field("outcome", outcome))
			out <- sponsoredResult{}
			return
		}
		sponsoredRequests.Inc("ok")
		out <- sponsoredResult{slots: resp.GetSlots()}
	}()
	return out
}

// placeSponsored 把页内相对槽位换算为 afterPosition；本页帖子不足时丢弃该槽位。帖子 items、
// position、游标与去重语义保持不变（ADS-020、DISC-053）。
func placeSponsored(items []types.RecommendFeedItem, slots []*adservice.SponsoredSlot) []types.SponsoredSlotItem {
	out := make([]types.SponsoredSlotItem, 0, len(slots))
	for _, slot := range slots {
		ad := slot.GetAd()
		index := int(slot.GetAfterIndex())
		if ad == nil || index < 1 || index > len(items) || strings.TrimSpace(slot.GetSlotId()) == "" {
			continue
		}
		why := ad.GetWhy()
		images := ad.GetImages()
		if images == nil {
			images = []string{}
		}
		out = append(out, types.SponsoredSlotItem{
			SlotId: slot.GetSlotId(), AfterPosition: items[index-1].Position,
			Ad: types.SponsoredAdItem{
				AdId: ad.GetAdId(), Revision: ad.GetRevision(), AdvertiserName: ad.GetAdvertiserName(), Title: ad.GetTitle(),
				Body: ad.GetBody(), Cta: ad.GetCta(), LandingUrl: ad.GetLandingUrl(), LandingDomain: ad.GetLandingDomain(),
				Images: images, Disclosure: sponsoredDisclosure,
				Why: types.SponsoredWhyItem{Market: why.GetMarket(), Scene: why.GetScene(), Personalized: why.GetPersonalized()},
			},
		})
	}
	return out
}
