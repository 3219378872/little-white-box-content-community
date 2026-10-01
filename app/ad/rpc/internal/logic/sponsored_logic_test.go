package logic

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
)

func indexEntry(t *testing.T, kv *fakeKV, market string, entry serving.Entry) {
	t.Helper()
	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	key := serving.IndexKey(market)
	if kv.hashes[key] == nil {
		kv.hashes[key] = map[string]string{}
	}
	kv.hashes[key][strconv.FormatInt(entry.AdID, 10)] = string(raw)
}

func snapshotFor(adID int64) *store.Snapshot {
	return &store.Snapshot{
		AdID: adID, Revision: 1, AdvertiserName: "Acme", Title: "Ad " + strconv.FormatInt(adID, 10), Body: "b",
		CTA: "Go", LandingURL: "https://shop.example.com", LandingDomain: "shop.example.com",
		MediaJSON: `[{"mediaId":3,"sha256":"abc"}]`, Market: "US", Industry: "GENERAL",
	}
}

func slotsReq() *pb.GetSponsoredSlotsReq {
	return &pb.GetSponsoredSlotsReq{UserId: 1, RequestId: "req-1", Cursor: "c1", PageItems: 20}
}

// sponsoredFixture 准备四条候选：1 可投；2 被隐藏；3 未到投放期；4 金融行业缺资质。
func sponsoredFixture(t *testing.T) (*fakeStore, *fakeKV, *GetSponsoredSlotsLogic) {
	t.Helper()
	fs := &fakeStore{snapshots: map[int64]*store.Snapshot{1: snapshotFor(1)}}
	svcCtx, _, kv := newSvcCtx(fs)
	now := testNow.UnixMilli()
	indexEntry(t, kv, "US", serving.Entry{AdID: 1, Revision: 1, AdvertiserID: 10, Industry: "GENERAL"})
	indexEntry(t, kv, "US", serving.Entry{AdID: 2, Revision: 1, AdvertiserID: 20, Industry: "GENERAL"})
	indexEntry(t, kv, "US", serving.Entry{AdID: 3, Revision: 1, AdvertiserID: 30, Industry: "GENERAL", StartMs: now + 1000})
	indexEntry(t, kv, "US", serving.Entry{AdID: 4, Revision: 1, AdvertiserID: 40, Industry: "FINANCIAL"})
	kv.hashes[serving.HiddenKey("u:1")] = map[string]string{"2": strconv.FormatInt(now+60_000, 10)}
	return fs, kv, NewGetSponsoredSlotsLogic(context.Background(), svcCtx)
}

func TestGetSponsoredSlotsServesOnlyEligibleAdsAndCachesTheRequest(t *testing.T) {
	fs, kv, l := sponsoredFixture(t)

	resp, err := l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err)
	require.Len(t, resp.Slots, 1)
	slot := resp.Slots[0]
	require.Equal(t, int64(1), slot.Ad.AdId)
	require.Equal(t, "Ad 1", slot.Ad.Title)
	require.Len(t, slot.Ad.Images, 1)
	require.Equal(t, &pb.SponsoredWhy{Market: "US", Scene: "home"}, slot.Ad.Why)
	require.Equal(t, 1, fs.calls["QualificationsOf"], "only the financial ad needs a qualification check")
	require.Equal(t, 1, kv.reserveCalls)

	again, err := l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err)
	require.Equal(t, resp.Slots[0].Ad.AdId, again.Slots[0].Ad.AdId)
	require.Equal(t, 1, kv.reserveCalls, "a retried request must not count frequency twice")
}

func TestGetSponsoredSlotsDropsCappedAndUnreadableAds(t *testing.T) {
	_, kv, l := sponsoredFixture(t)
	kv.capped["1"] = true
	resp, err := l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err)
	require.Empty(t, resp.Slots)

	fs, _, l := sponsoredFixture(t)
	fs.snapshotErr = errBoom
	resp, err = l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err)
	require.Empty(t, resp.Slots)
}

func TestGetSponsoredSlotsRequestValidation(t *testing.T) {
	_, _, l := sponsoredFixture(t)

	resp, err := l.GetSponsoredSlots(&pb.GetSponsoredSlotsReq{RequestId: "r", PageItems: 20})
	require.NoError(t, err, "no identity degrades to no ads")
	require.Empty(t, resp.Slots)

	resp, err = l.GetSponsoredSlots(&pb.GetSponsoredSlotsReq{UserId: 1, PageItems: 20})
	require.NoError(t, err)
	require.Empty(t, resp.Slots)

	req := slotsReq()
	req.Market = "xx"
	_, err = l.GetSponsoredSlots(req)
	requireCode(t, err, errx.ParamError)
}

func TestGetSponsoredSlotsSurfacesRedisFailures(t *testing.T) {
	cases := map[string]func(*fakeKV){
		"request cache": func(kv *fakeKV) { kv.getErr = errBoom },
		"index":         func(kv *fakeKV) { kv.hgetallErr = errBoom },
		"reserve":       func(kv *fakeKV) { kv.evalErr = errBoom },
	}
	for name, breakKV := range cases {
		t.Run(name, func(t *testing.T) {
			_, kv, l := sponsoredFixture(t)
			breakKV(kv)
			_, err := l.GetSponsoredSlots(slotsReq())
			requireCode(t, err, errx.SystemError)
		})
	}

	_, kv, l := sponsoredFixture(t)
	kv.setErr = errBoom
	resp, err := l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err, "failing to cache the response must not fail the request")
	require.Len(t, resp.Slots, 1)
}

func TestGetSponsoredSlotsServesQualifiedRegulatedAds(t *testing.T) {
	fs, _, l := sponsoredFixture(t)
	fs.snapshots[4] = snapshotFor(4)
	fs.quals = map[int64][]store.Qualification{40: {{
		Market: "US", Industry: "FINANCIAL", Status: store.QualificationApproved, ValidUntilMs: testNow.UnixMilli() + 1,
	}}}

	resp, err := l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err)
	var ids []int64
	for _, slot := range resp.Slots {
		ids = append(ids, slot.Ad.AdId)
	}
	require.ElementsMatch(t, []int64{1, 4}, ids)

	fs, _, l = sponsoredFixture(t)
	fs.qualsErr = errBoom
	resp, err = l.GetSponsoredSlots(slotsReq())
	require.NoError(t, err, "unknown qualification means the regulated ad is skipped")
	require.Len(t, resp.Slots, 1)
}

func TestHideAd(t *testing.T) {
	fs := &fakeStore{}
	svcCtx, _, kv := newSvcCtx(fs)
	l := NewHideAdLogic(context.Background(), svcCtx)

	_, err := l.HideAd(&pb.HideAdReq{AdId: 7})
	requireCode(t, err, errx.ParamError)
	_, err = l.HideAd(&pb.HideAdReq{UserId: 1})
	requireCode(t, err, errx.ParamError)

	_, err = l.HideAd(&pb.HideAdReq{UserId: 1, AdId: 7})
	require.NoError(t, err)
	require.Contains(t, kv.hashes[serving.HiddenKey("u:1")], "7")

	kv.evalErr = errBoom
	_, err = l.HideAd(&pb.HideAdReq{SessionId: "anon", AdId: 7})
	requireCode(t, err, errx.SystemError)
}
