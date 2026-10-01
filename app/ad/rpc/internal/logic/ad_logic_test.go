package logic

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/ad/internal/store"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
)

func validAdInput() *pb.AdInput {
	return &pb.AdInput{
		Title: " Spring sale ", Body: "Everything 20% off", Cta: "Shop", LandingUrl: "https://shop.example.com/sale",
		MediaIds: []int64{3, 3, 4}, Market: "us", Industry: "general",
	}
}

func servingAd() *store.AdWithSnapshots {
	approved := &store.Snapshot{
		AdID: 7, Revision: 1, AdvertiserName: "Acme", Title: "Approved", Body: "b", CTA: "c",
		LandingURL: "https://shop.example.com", LandingDomain: "shop.example.com",
		MediaJSON: `[{"mediaId":3,"sha256":"abc"}]`, Market: "US", Language: "en", Industry: "GENERAL",
	}
	latest := *approved
	latest.Revision, latest.Title = 2, "Draft"
	return &store.AdWithSnapshots{
		Ad: &store.Ad{
			ID: 7, AdvertiserID: 10, Revision: 2, ApprovedRevision: 1, PublishedRevision: 1,
			ReviewStatus: "pending", ServingStatus: store.ServingServing, PolicyCodesJSON: `[]`,
		},
		Latest: &latest, Approved: approved,
	}
}

func TestValidateAdInput(t *testing.T) {
	cases := map[string]struct {
		mutate func(*pb.AdInput)
		key    string
		code   int
	}{
		"nil input":        {mutate: nil, code: errx.ParamError},
		"long idempotency": {mutate: func(*pb.AdInput) {}, key: strings.Repeat("k", 129), code: errx.ParamError},
		"empty title":      {mutate: func(in *pb.AdInput) { in.Title = " " }, code: errx.ParamError},
		"long body":        {mutate: func(in *pb.AdInput) { in.Body = strings.Repeat("b", 501) }, code: errx.ParamError},
		"long cta":         {mutate: func(in *pb.AdInput) { in.Cta = strings.Repeat("c", 33) }, code: errx.ParamError},
		"unknown market":   {mutate: func(in *pb.AdInput) { in.Market = "XX" }, code: errx.AdIndustryUnsupported},
		"unknown industry": {mutate: func(in *pb.AdInput) { in.Industry = "TOYS" }, code: errx.AdIndustryUnsupported},
		"http landing":     {mutate: func(in *pb.AdInput) { in.LandingUrl = "http://shop.example.com" }, code: errx.AdLandingInvalid},
		"negative start":   {mutate: func(in *pb.AdInput) { in.StartMs = -1 }, code: errx.ParamError},
		"end before start": {mutate: func(in *pb.AdInput) { in.StartMs, in.EndMs = 10, 10 }, code: errx.ParamError},
		"non-positive id":  {mutate: func(in *pb.AdInput) { in.MediaIds = []int64{0} }, code: errx.ParamError},
		"too many media":   {mutate: func(in *pb.AdInput) { in.MediaIds = []int64{1, 2, 3, 4} }, code: errx.ParamError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var in *pb.AdInput
			if tc.mutate != nil {
				in = validAdInput()
				tc.mutate(in)
			}
			_, err := validateAdInput(in, tc.key)
			requireCode(t, err, tc.code)
		})
	}

	got, err := validateAdInput(validAdInput(), "key")
	require.NoError(t, err)
	require.Equal(t, "Spring sale", got.Title)
	require.Equal(t, []int64{3, 4}, got.MediaIDs)
	require.Equal(t, "US", got.Market)
	require.Equal(t, "GENERAL", got.Industry)
	require.Equal(t, "shop.example.com", got.LandingDomain)
}

func TestCreateAdReturnsView(t *testing.T) {
	fs := &fakeStore{ad: servingAd()}
	svcCtx, _, _ := newSvcCtx(fs)
	resp, err := NewCreateAdLogic(context.Background(), svcCtx).CreateAd(&pb.CreateAdReq{UserId: 1, Input: validAdInput()})
	require.NoError(t, err)
	require.Equal(t, int64(7), resp.Ad.AdId)
	require.Equal(t, "Draft", resp.Ad.Latest.Title)
	require.Empty(t, resp.Ad.Latest.Media[0].PublicUrl, "the unpublished draft never exposes a public URL")
	require.Equal(t, "Approved", resp.Ad.Approved.Title)
	require.Contains(t, resp.Ad.Approved.Media[0].PublicUrl, "https://cdn.example.test/xbh-media")
	require.True(t, resp.Ad.Eligible)
	require.Equal(t, []int64{3, 4}, fs.adInput.MediaIDs)
}

func TestCreateAdFailures(t *testing.T) {
	ctx := context.Background()
	svcCtx, _, _ := newSvcCtx(&fakeStore{})
	_, err := NewCreateAdLogic(ctx, svcCtx).CreateAd(&pb.CreateAdReq{UserId: 1})
	requireCode(t, err, errx.ParamError)

	svcCtx, _, _ = newSvcCtx(&fakeStore{err: store.ErrAdvertiserNotApproved})
	_, err = NewCreateAdLogic(ctx, svcCtx).CreateAd(&pb.CreateAdReq{UserId: 1, Input: validAdInput()})
	requireCode(t, err, errx.AdvertiserRequired)

	svcCtx, _, _ = newSvcCtx(&fakeStore{ad: servingAd(), qualsErr: errBoom})
	_, err = NewCreateAdLogic(ctx, svcCtx).CreateAd(&pb.CreateAdReq{UserId: 1, Input: validAdInput()})
	requireCode(t, err, errx.SystemError)
}

func TestUpdateAd(t *testing.T) {
	ctx := context.Background()
	svcCtx, _, _ := newSvcCtx(&fakeStore{ad: servingAd()})
	for _, req := range []*pb.UpdateAdReq{
		{UserId: 1, AdId: 0, ExpectedRevision: 1, Input: validAdInput()},
		{UserId: 1, AdId: 7, ExpectedRevision: 0, Input: validAdInput()},
		{UserId: 1, AdId: 7, ExpectedRevision: 1},
	} {
		_, err := NewUpdateAdLogic(ctx, svcCtx).UpdateAd(req)
		requireCode(t, err, errx.ParamError)
	}

	resp, err := NewUpdateAdLogic(ctx, svcCtx).UpdateAd(&pb.UpdateAdReq{UserId: 1, AdId: 7, ExpectedRevision: 1, Input: validAdInput()})
	require.NoError(t, err)
	require.Equal(t, int64(2), resp.Ad.Revision)

	svcCtx, _, _ = newSvcCtx(&fakeStore{err: store.ErrRevisionConflict})
	_, err = NewUpdateAdLogic(ctx, svcCtx).UpdateAd(&pb.UpdateAdReq{UserId: 1, AdId: 7, ExpectedRevision: 1, Input: validAdInput()})
	requireCode(t, err, errx.ContentVersionConflict)
}

func TestGetAd(t *testing.T) {
	ctx := context.Background()
	svcCtx, _, _ := newSvcCtx(&fakeStore{err: store.ErrNotFound})
	_, err := NewGetAdLogic(ctx, svcCtx).GetAd(&pb.GetAdReq{UserId: 1, AdId: 7})
	requireCode(t, err, errx.NotFound)

	svcCtx, _, _ = newSvcCtx(&fakeStore{ad: servingAd()})
	resp, err := NewGetAdLogic(ctx, svcCtx).GetAd(&pb.GetAdReq{UserId: 1, AdId: 7})
	require.NoError(t, err)
	require.True(t, resp.Ad.Eligible)
}

func TestListAdsLoadsQualificationsOncePerAdvertiser(t *testing.T) {
	second := servingAd()
	second.Ad.ID = 8
	other := servingAd()
	other.Ad.ID, other.Ad.AdvertiserID = 9, 11
	fs := &fakeStore{ads: []store.AdWithSnapshots{*servingAd(), *second, *other}}
	svcCtx, _, _ := newSvcCtx(fs)

	resp, err := NewListAdsLogic(context.Background(), svcCtx).ListAds(&pb.ListAdsReq{UserId: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, resp.Ads, 3)
	require.Equal(t, int64(42), resp.NextCursor)
	require.True(t, resp.HasMore)
	require.Equal(t, 2, fs.calls["QualificationsOf"])

	fs.qualsErr = errBoom
	_, err = NewListAdsLogic(context.Background(), svcCtx).ListAds(&pb.ListAdsReq{UserId: 1})
	requireCode(t, err, errx.SystemError)

	svcCtx, _, _ = newSvcCtx(&fakeStore{err: errBoom})
	_, err = NewListAdsLogic(context.Background(), svcCtx).ListAds(&pb.ListAdsReq{UserId: 1})
	requireCode(t, err, errx.SystemError)
}

func TestEligibleRequiresApprovedServingPublishedAndQualified(t *testing.T) {
	now := testNow.UnixMilli()
	quals := []store.Qualification{{Market: "US", Industry: "FINANCIAL", Status: store.QualificationApproved, ValidUntilMs: now + 1}}
	cases := map[string]struct {
		mutate func(*store.Ad, *store.Snapshot) *store.Snapshot
		quals  []store.Qualification
		want   bool
	}{
		"serving":           {want: true},
		"never approved":    {mutate: func(a *store.Ad, s *store.Snapshot) *store.Snapshot { a.ApprovedRevision = 0; return s }},
		"no approved snap":  {mutate: func(_ *store.Ad, _ *store.Snapshot) *store.Snapshot { return nil }},
		"paused":            {mutate: func(a *store.Ad, s *store.Snapshot) *store.Snapshot { a.ServingStatus = "paused"; return s }},
		"not started":       {mutate: func(a *store.Ad, s *store.Snapshot) *store.Snapshot { a.StartMs = now + 1; return s }},
		"ended":             {mutate: func(a *store.Ad, s *store.Snapshot) *store.Snapshot { a.EndMs = now; return s }},
		"not yet published": {mutate: func(a *store.Ad, s *store.Snapshot) *store.Snapshot { a.PublishedRevision = 0; return s }},
		"missing qualification": {mutate: func(_ *store.Ad, s *store.Snapshot) *store.Snapshot {
			s.Industry = "FINANCIAL"
			return s
		}},
		"valid qualification": {quals: quals, want: true, mutate: func(_ *store.Ad, s *store.Snapshot) *store.Snapshot {
			s.Industry = "FINANCIAL"
			return s
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			full := servingAd()
			approved := full.Approved
			if tc.mutate != nil {
				approved = tc.mutate(full.Ad, approved)
			}
			require.Equal(t, tc.want, eligible(full.Ad, approved, tc.quals, now))
		})
	}
}
