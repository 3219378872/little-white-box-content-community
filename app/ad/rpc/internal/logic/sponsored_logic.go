package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/metrics"
	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/app/ad/rpc/internal/svc"
	pb "esx/kitex_gen/ad"
	"esx/pkg/adpolicy"
	"esx/pkg/errx"
	redis "esx/pkg/redisstore"
)

// DefaultMarket 是未声明市场时使用的演示市场。
const DefaultMarket = "US"

// GetSponsoredSlotsLogic 承载 GetSponsoredSlots 接口的业务逻辑；每个请求新建一个实例。
type GetSponsoredSlotsLogic struct{ base }

// NewGetSponsoredSlotsLogic 绑定请求上下文与服务依赖。
func NewGetSponsoredSlotsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSponsoredSlotsLogic {
	return &GetSponsoredSlotsLogic{newBase(ctx, svcCtx)}
}

// cachedSlot 是缓存在 Redis 中的一个已选槽位，重试时原样返回。
type cachedSlot struct {
	SlotID     string
	AfterIndex int32
	Ad         cachedAd
}

// cachedAd 是槽位中展示的广告内容。
type cachedAd struct {
	AdID, Revision                                int64
	AdvertiserName, Title, Body, CTA, URL, Domain string
	Images                                        []string
	Market, Scene                                 string
}

// GetSponsoredSlots 为一页推荐流选择广告槽位（ADS-020～ADS-025、ADS-041）。
// 同一（请求，游标，身份）重试返回相同结果且不重复计频；Redis 不可用时返回错误，由 Gateway 降级为无广告。
func (l *GetSponsoredSlotsLogic) GetSponsoredSlots(in *pb.GetSponsoredSlotsReq) (*pb.GetSponsoredSlotsResp, error) {
	identity, err := serving.Identity(in.GetUserId(), strings.TrimSpace(in.GetSessionId()))
	requestID := strings.TrimSpace(in.GetRequestId())
	if err != nil || requestID == "" || in.GetPageItems() <= 0 {
		metrics.Request("no-identity")
		return &pb.GetSponsoredSlotsResp{}, nil
	}
	market := strings.ToUpper(strings.TrimSpace(in.GetMarket()))
	if market == "" {
		market = DefaultMarket
	}
	if !adpolicy.IsMarket(market) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	scene := strings.TrimSpace(in.GetScene())
	if scene == "" {
		scene = "home"
	}
	now := l.svcCtx.Now()
	kv := l.svcCtx.Redis
	// 同一（请求，游标，身份，市场）已选过槽位时直接返回缓存，重试不会再次计频。
	reqKey := serving.RequestKey(requestID, in.GetCursor()+"|"+identity+"|"+market)
	if raw, err := kv.GetCtx(l.ctx, reqKey); err == nil && raw != "" {
		var cached []cachedSlot
		if json.Unmarshal([]byte(raw), &cached) == nil {
			metrics.Request("cached")
			return toResponse(cached), nil
		}
	} else if err != nil && !errors.Is(err, redis.Nil) {
		metrics.Degraded("redis", "request-cache")
		return nil, l.mapError(err, "read slot cache")
	}

	candidates, err := l.candidates(market, identity, now)
	if err != nil {
		metrics.Degraded("redis", "candidates")
		return nil, l.mapError(err, "load candidates")
	}
	// 先选出候选，再原子预留频次；未获得频次的广告不下发。
	choices := serving.Choose(requestID, int(in.GetPageItems()), candidates)
	ids := make([]int64, 0, len(choices))
	for _, c := range choices {
		ids = append(ids, c.Entry.AdID)
	}
	granted, err := serving.Reserve(l.ctx, kv, identity, ids, now)
	if err != nil {
		metrics.Degraded("redis", "reserve")
		return nil, l.mapError(err, "reserve frequency")
	}
	allowed := map[int64]bool{}
	for _, id := range granted {
		allowed[id] = true
	}
	var slots []cachedSlot
	for _, choice := range choices {
		if !allowed[choice.Entry.AdID] {
			metrics.Capped(1)
			continue
		}
		ad, err := l.content(choice.Entry, market, scene, now)
		if err != nil {
			metrics.Degraded("store", "snapshot")
			continue
		}
		slots = append(slots, cachedSlot{SlotID: choice.SlotID, AfterIndex: int32(choice.AfterIndex), Ad: ad})
	}
	// 缓存写入失败只降级为重试时可能重新选择，不影响本次响应。
	if raw, err := json.Marshal(slots); err == nil {
		if err := kv.SetexCtx(l.ctx, reqKey, string(raw), int(serving.RequestTTL.Seconds())); err != nil {
			metrics.Degraded("redis", "request-cache-write")
		}
	}
	if len(slots) == 0 {
		metrics.Request("empty")
	} else {
		metrics.Request("served")
		metrics.Slots(market, len(slots))
	}
	return toResponse(slots), nil
}

// candidates 过滤索引：投放期、隐藏、资质有效（ADS-002、ADS-010、ADS-026）。
func (l *GetSponsoredSlotsLogic) candidates(market, identity string, now time.Time) ([]serving.Candidate, error) {
	entries, err := l.index(market, now)
	if err != nil {
		return nil, err
	}
	hidden, err := serving.Hidden(l.ctx, l.svcCtx.Redis, identity, now)
	if err != nil {
		return nil, err
	}
	counts, err := serving.Counts(l.ctx, l.svcCtx.Redis, identity, now)
	if err != nil {
		return nil, err
	}
	var out []serving.Candidate
	for _, entry := range entries {
		if hidden[entry.AdID] || !entry.InWindow(now.UnixMilli()) {
			continue
		}
		if adpolicy.DispositionOf(market, entry.Industry).NeedsQualification && !l.qualified(entry, market, now) {
			continue
		}
		out = append(out, serving.Candidate{Entry: entry, TodayCount: counts[entry.AdID]})
	}
	return out, nil
}

// index 读取某市场的投放索引，经进程内短期缓存。
func (l *GetSponsoredSlotsLogic) index(market string, now time.Time) ([]serving.Entry, error) {
	key := "index:" + market
	if cached, ok := l.svcCtx.Cache.Get(key, now); ok {
		return cached.([]serving.Entry), nil
	}
	entries, err := l.svcCtx.Index.Load(l.ctx, market)
	if err != nil {
		return nil, err
	}
	l.svcCtx.Cache.Put(key, entries, now)
	return entries, nil
}

// qualified 判断广告主在该市场与行业是否有有效资质，资质列表经进程内短期缓存。
func (l *GetSponsoredSlotsLogic) qualified(entry serving.Entry, market string, now time.Time) bool {
	key := "quals:" + strconv.FormatInt(entry.AdvertiserID, 10)
	quals, ok := l.svcCtx.Cache.Get(key, now)
	if !ok {
		loaded, err := l.svcCtx.Store.QualificationsOf(l.ctx, entry.AdvertiserID)
		if err != nil {
			return false // 无法确认资格时不投放（ADS-022）
		}
		l.svcCtx.Cache.Put(key, loaded, now)
		quals = loaded
	}
	return store.ValidQualification(quals.([]store.Qualification), market, entry.Industry, now.UnixMilli())
}

// content 只从过审快照取投放内容（ADS-012）；why 只含市场、场景与非个性化标记（ADS-023、ADS-025）。
func (l *GetSponsoredSlotsLogic) content(entry serving.Entry, market, scene string, now time.Time) (cachedAd, error) {
	key := "snap:" + strconv.FormatInt(entry.AdID, 10) + ":" + strconv.FormatInt(entry.Revision, 10)
	cached, ok := l.svcCtx.Cache.Get(key, now)
	if !ok {
		snap, err := l.svcCtx.Store.SnapshotAt(l.ctx, entry.AdID, entry.Revision)
		if err != nil {
			return cachedAd{}, err
		}
		l.svcCtx.Cache.Put(key, snap, now)
		cached = snap
	}
	snap := cached.(*store.Snapshot)
	ad := cachedAd{
		AdID: entry.AdID, Revision: entry.Revision, AdvertiserName: snap.AdvertiserName, Title: snap.Title,
		Body: snap.Body, CTA: snap.CTA, URL: snap.LandingURL, Domain: snap.LandingDomain, Market: market, Scene: scene,
	}
	for _, m := range snap.Media() {
		ad.Images = append(ad.Images, assets.PublicURLFor(l.svcCtx.Config.Assets.PublicBaseURL, m.SHA256))
	}
	return ad, nil
}

// toResponse 把缓存槽位转换为响应；why 固定为非个性化（ADS-025）。
func toResponse(slots []cachedSlot) *pb.GetSponsoredSlotsResp {
	resp := &pb.GetSponsoredSlotsResp{}
	for _, s := range slots {
		resp.Slots = append(resp.Slots, &pb.SponsoredSlot{
			SlotId: s.SlotID, AfterIndex: s.AfterIndex,
			Ad: &pb.SponsoredAd{
				AdId: s.Ad.AdID, Revision: s.Ad.Revision, AdvertiserName: s.Ad.AdvertiserName, Title: s.Ad.Title,
				Body: s.Ad.Body, Cta: s.Ad.CTA, LandingUrl: s.Ad.URL, LandingDomain: s.Ad.Domain, Images: s.Ad.Images,
				Why: &pb.SponsoredWhy{Market: s.Ad.Market, Scene: s.Ad.Scene, Personalized: false},
			},
		})
	}
	return resp
}

// HideAdLogic 承载 HideAd 接口的业务逻辑；每个请求新建一个实例。
type HideAdLogic struct{ base }

// NewHideAdLogic 绑定请求上下文与服务依赖。
func NewHideAdLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HideAdLogic {
	return &HideAdLogic{newBase(ctx, svcCtx)}
}

// HideAd 记录隐藏（ADS-026）：已认证用户 30 天；匿名用户只在当前会话内生效。
func (l *HideAdLogic) HideAd(in *pb.HideAdReq) (*pb.HideAdResp, error) {
	identity, err := serving.Identity(in.GetUserId(), strings.TrimSpace(in.GetSessionId()))
	if err != nil || in.GetAdId() <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err := serving.Hide(l.ctx, l.svcCtx.Redis, identity, in.GetAdId(), l.svcCtx.Now()); err != nil {
		return nil, l.mapError(err, "hide ad")
	}
	return &pb.HideAdResp{}, nil
}
