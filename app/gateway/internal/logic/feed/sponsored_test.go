package feed

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/stretchr/testify/require"

	"esx/app/ad/rpc/adservice"
	"esx/app/feed/rpc/feedservice"
	"esx/app/gateway/internal/config"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	adpb "esx/kitex_gen/ad"
	feedpb "esx/kitex_gen/feed"
)

type fakeAdService struct {
	adservice.AdService
	calls int32
	fn    func(context.Context, *adservice.GetSponsoredSlotsReq) (*adservice.GetSponsoredSlotsResp, error)
}

func (f *fakeAdService) GetSponsoredSlots(ctx context.Context, in *adservice.GetSponsoredSlotsReq, _ ...callopt.Option) (*adservice.GetSponsoredSlotsResp, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.fn(ctx, in)
}

func recommendSvc(ads *fakeAdService) *svc.ServiceContext {
	items := make([]*feedpb.FeedItem, 0, 6)
	for i := range 6 {
		items = append(items, &feedpb.FeedItem{PostId: int64(100 + i), AuthorId: 1, Position: int32(21 + i)})
	}
	return &svc.ServiceContext{
		Config: config.Config{SponsoredTimeoutMs: 50},
		FeedService: &fakeFeedService{getRecommendFeedFn: func(context.Context, *feedservice.GetRecommendFeedReq, ...callopt.Option) (*feedservice.GetRecommendFeedResp, error) {
			return &feedservice.GetRecommendFeedResp{Items: items, NextCursor: "c2", HasMore: true, RequestId: "r1"}, nil
		}},
		UserService: &fakeUserService{batchGetUsersFn: func(context.Context, *userservice.BatchGetUsersReq, ...callopt.Option) (*userservice.BatchGetUsersResp, error) {
			return &userservice.BatchGetUsersResp{}, nil
		}},
		AdService: ads,
	}
}

func slot(id string, afterIndex int32) *adpb.SponsoredSlot {
	return &adpb.SponsoredSlot{SlotId: id, AfterIndex: afterIndex, Ad: &adpb.SponsoredAd{
		AdId: 7, Revision: 2, AdvertiserName: "Acme", Title: "Beans", LandingUrl: "https://coffee.shop.com",
		LandingDomain: "coffee.shop.com", Why: &adpb.SponsoredWhy{Market: "US", Scene: "home"},
	}}
}

func recommend(t *testing.T, svcCtx *svc.ServiceContext, adSlots int32) (*types.GetRecommendFeedResp, string) {
	t.Helper()
	resp, err := NewGetRecommendFeedLogic(context.Background(), svcCtx).GetRecommendFeed(&types.GetRecommendFeedReq{
		AnonymousId: "anon", SessionId: "sess", RequestId: "r1", PageSize: 6, AdSlots: adSlots,
	})
	require.NoError(t, err)
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	return resp, string(raw)
}

// ADS-A01：未声明能力的请求不查询广告，响应中不出现 sponsored 字段。
func TestRecommendWithoutAdSlotsIsUnchanged(t *testing.T) {
	ads := &fakeAdService{fn: func(context.Context, *adservice.GetSponsoredSlotsReq) (*adservice.GetSponsoredSlotsResp, error) {
		return &adservice.GetSponsoredSlotsResp{Slots: []*adpb.SponsoredSlot{slot("s1", 4)}}, nil
	}}
	resp, raw := recommend(t, recommendSvc(ads), 0)
	require.Len(t, resp.Items, 6)
	require.NotContains(t, raw, "sponsored")
	require.Zero(t, atomic.LoadInt32(&ads.calls))
}

// ADS-A01 / DISC-053：声明后获得带标识的槽位，afterPosition 取本页帖子的 position，帖子条目不变。
func TestRecommendWithAdSlotsPlacesSlotsByPosition(t *testing.T) {
	var got *adservice.GetSponsoredSlotsReq
	ads := &fakeAdService{fn: func(_ context.Context, in *adservice.GetSponsoredSlotsReq) (*adservice.GetSponsoredSlotsResp, error) {
		got = in
		return &adservice.GetSponsoredSlotsResp{Slots: []*adpb.SponsoredSlot{slot("s1", 4), slot("s2", 12)}}, nil
	}}
	resp, raw := recommend(t, recommendSvc(ads), 1)
	require.Len(t, resp.Items, 6)
	require.Len(t, resp.Sponsored, 1) // 第 12 条之后的槽位因本页不足被丢弃
	require.Equal(t, int32(24), resp.Sponsored[0].AfterPosition)
	require.Equal(t, "sponsored", resp.Sponsored[0].Ad.Disclosure)
	require.False(t, resp.Sponsored[0].Ad.Why.Personalized)
	require.True(t, strings.Contains(raw, `"sponsored"`))
	require.Equal(t, "sess", got.SessionId)
	require.Zero(t, got.UserId)
	require.Equal(t, int32(6), got.PageItems)
}

// ADS-A03 / ADS-022：广告服务失败或超时时推荐流照常成功且不含广告。
func TestRecommendDegradesWhenAdServiceFails(t *testing.T) {
	failing := &fakeAdService{fn: func(context.Context, *adservice.GetSponsoredSlotsReq) (*adservice.GetSponsoredSlotsResp, error) {
		return nil, errors.New("ad-rpc down")
	}}
	resp, raw := recommend(t, recommendSvc(failing), 1)
	require.Len(t, resp.Items, 6)
	require.Empty(t, resp.Sponsored)
	require.NotContains(t, raw, "sponsored")

	slow := &fakeAdService{fn: func(ctx context.Context, _ *adservice.GetSponsoredSlotsReq) (*adservice.GetSponsoredSlotsResp, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return &adservice.GetSponsoredSlotsResp{Slots: []*adpb.SponsoredSlot{slot("s1", 4)}}, nil
		}
	}}
	started := time.Now()
	resp, _ = recommend(t, recommendSvc(slow), 1)
	require.Len(t, resp.Items, 6)
	require.Empty(t, resp.Sponsored)
	require.Less(t, time.Since(started), time.Second)
}

func TestPlaceSponsoredDropsMalformedSlots(t *testing.T) {
	items := []types.RecommendFeedItem{{Position: 1}, {Position: 2}}
	slots := []*adpb.SponsoredSlot{slot("", 1), slot("s2", 0), {SlotId: "s3", AfterIndex: 1}, slot("s4", 2)}
	placed := placeSponsored(items, slots)
	require.Len(t, placed, 1)
	require.Equal(t, "s4", placed[0].SlotId)
	require.Equal(t, int32(2), placed[0].AfterPosition)
	require.NotNil(t, placed[0].Ad.Images)
}

func TestInvalidMarketSkipsAds(t *testing.T) {
	ads := &fakeAdService{fn: func(context.Context, *adservice.GetSponsoredSlotsReq) (*adservice.GetSponsoredSlotsResp, error) {
		return &adservice.GetSponsoredSlotsResp{}, nil
	}}
	result := <-startSponsored(context.Background(), recommendSvc(ads), sponsoredRequest{market: "CN"})
	require.Empty(t, result.slots)
	require.Zero(t, atomic.LoadInt32(&ads.calls))
}
