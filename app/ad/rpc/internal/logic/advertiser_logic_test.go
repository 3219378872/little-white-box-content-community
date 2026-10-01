package logic

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/ad/internal/assets"
	"esx/app/ad/internal/store"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
	"esx/pkg/idempotencyx"
)

func sampleAdvertiser() *store.AdvertiserWithQualifications {
	return &store.AdvertiserWithQualifications{
		Advertiser: &store.Advertiser{
			ID: 10, UserID: 1, Name: "Acme", MarketsCSV: "US,DE", Revision: 2, ApprovedRevision: 1,
			ReviewStatus: "approved", PolicyCodesJSON: `["CONTENT.IP"]`, UpdatedAtMs: 99,
		},
		Qualifications: []store.Qualification{{
			ID: 5, Market: "US", Industry: "FINANCIAL", DocumentMediaID: 8, ValidUntilMs: 1e15,
			Status: store.QualificationApproved, SubmittedRevision: 2,
		}},
	}
}

func TestLogicsRequireLogin(t *testing.T) {
	ctx := context.Background()
	svcCtx, _, _ := newSvcCtx(&fakeStore{})
	calls := map[string]func() error{
		"ApplyAdvertiser": func() error {
			_, err := NewApplyAdvertiserLogic(ctx, svcCtx).ApplyAdvertiser(&pb.ApplyAdvertiserReq{})
			return err
		},
		"GetMyAdvertiser": func() error {
			_, err := NewGetMyAdvertiserLogic(ctx, svcCtx).GetMyAdvertiser(&pb.GetMyAdvertiserReq{})
			return err
		},
		"AddQualification": func() error {
			_, err := NewAddQualificationLogic(ctx, svcCtx).AddQualification(&pb.AddQualificationReq{})
			return err
		},
		"CreateAd": func() error { _, err := NewCreateAdLogic(ctx, svcCtx).CreateAd(&pb.CreateAdReq{}); return err },
		"UpdateAd": func() error { _, err := NewUpdateAdLogic(ctx, svcCtx).UpdateAd(&pb.UpdateAdReq{}); return err },
		"GetAd":    func() error { _, err := NewGetAdLogic(ctx, svcCtx).GetAd(&pb.GetAdReq{}); return err },
		"ListAds":  func() error { _, err := NewListAdsLogic(ctx, svcCtx).ListAds(&pb.ListAdsReq{}); return err },
		"UploadAsset": func() error {
			_, err := NewUploadAssetLogic(ctx, svcCtx).UploadAsset(&pb.UploadAssetReq{})
			return err
		},
		"ReadAsset": func() error { _, err := NewReadAssetLogic(ctx, svcCtx).ReadAsset(&pb.ReadAssetReq{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) { requireCode(t, call(), errx.LoginRequired) })
	}
}

func TestMapErrorTranslatesStoreAndAssetErrors(t *testing.T) {
	cases := map[error]int{
		store.ErrNotFound:                   errx.NotFound,
		store.ErrRevisionConflict:           errx.ContentVersionConflict,
		store.ErrAdvertiserExists:           errx.AdvertiserExists,
		store.ErrAdvertiserNotApproved:      errx.AdvertiserRequired,
		store.ErrQualificationMissing:       errx.AdQualificationRequired,
		store.ErrAssetInvalid:               errx.AdMediaInvalid,
		store.ErrMarketNotAllowed:           errx.AdIndustryUnsupported,
		idempotencyx.ErrIdempotencyConflict: errx.IdempotencyConflict,
		assets.ErrTooLarge:                  errx.FileTooLarge,
		assets.ErrTypeNotAllowed:            errx.FileTypeNotAllowed,
		assets.ErrEmpty:                     errx.ParamError,
		errBoom:                             errx.SystemError,
	}
	b := newBase(context.Background(), nil)
	for in, code := range cases {
		requireCode(t, b.mapError(in, "test"), code)
	}
	require.NoError(t, b.mapError(nil, "test"))
}

func TestApplyAdvertiser(t *testing.T) {
	valid := func() *pb.ApplyAdvertiserReq {
		return &pb.ApplyAdvertiserReq{UserId: 1, Name: "  Acme  ", Markets: []string{"us", " DE ", "US"}}
	}
	invalid := map[string]func(*pb.ApplyAdvertiserReq){
		"unknown market":   func(r *pb.ApplyAdvertiserReq) { r.Markets = []string{"XX"} },
		"no market":        func(r *pb.ApplyAdvertiserReq) { r.Markets = nil },
		"name too short":   func(r *pb.ApplyAdvertiserReq) { r.Name = "A" },
		"name too long":    func(r *pb.ApplyAdvertiserReq) { r.Name = strings.Repeat("名", 65) },
		"negative rev":     func(r *pb.ApplyAdvertiserReq) { r.ExpectedRevision = -1 },
		"idempotency long": func(r *pb.ApplyAdvertiserReq) { r.IdempotencyKey = strings.Repeat("k", 129) },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			fs := &fakeStore{}
			svcCtx, _, _ := newSvcCtx(fs)
			req := valid()
			mutate(req)
			_, err := NewApplyAdvertiserLogic(context.Background(), svcCtx).ApplyAdvertiser(req)
			requireCode(t, err, errx.ParamError)
			require.Zero(t, fs.calls["ApplyAdvertiser"])
		})
	}

	t.Run("success normalizes input and maps the view", func(t *testing.T) {
		fs := &fakeStore{advertiser: sampleAdvertiser()}
		svcCtx, _, _ := newSvcCtx(fs)
		resp, err := NewApplyAdvertiserLogic(context.Background(), svcCtx).ApplyAdvertiser(valid())
		require.NoError(t, err)
		require.Equal(t, "Acme", fs.advertiserInput.Name)
		require.Equal(t, []string{"US", "DE"}, fs.advertiserInput.Markets)
		require.True(t, resp.Found)
		require.Equal(t, int64(10), resp.Advertiser.AdvertiserId)
		require.Equal(t, []string{"US", "DE"}, resp.Advertiser.Markets)
		require.Equal(t, []string{"CONTENT.IP"}, resp.Advertiser.PolicyCodes)
		require.Len(t, resp.Advertiser.Qualifications, 1)
		require.Equal(t, "FINANCIAL", resp.Advertiser.Qualifications[0].Industry)
	})

	t.Run("store conflict is mapped", func(t *testing.T) {
		svcCtx, _, _ := newSvcCtx(&fakeStore{err: store.ErrAdvertiserExists})
		_, err := NewApplyAdvertiserLogic(context.Background(), svcCtx).ApplyAdvertiser(valid())
		requireCode(t, err, errx.AdvertiserExists)
	})
}

func TestGetMyAdvertiser(t *testing.T) {
	ctx := context.Background()
	svcCtx, _, _ := newSvcCtx(&fakeStore{err: store.ErrNotFound})
	resp, err := NewGetMyAdvertiserLogic(ctx, svcCtx).GetMyAdvertiser(&pb.GetMyAdvertiserReq{UserId: 1})
	require.NoError(t, err)
	require.False(t, resp.Found)

	svcCtx, _, _ = newSvcCtx(&fakeStore{err: errBoom})
	_, err = NewGetMyAdvertiserLogic(ctx, svcCtx).GetMyAdvertiser(&pb.GetMyAdvertiserReq{UserId: 1})
	requireCode(t, err, errx.SystemError)

	svcCtx, _, _ = newSvcCtx(&fakeStore{advertiser: sampleAdvertiser()})
	resp, err = NewGetMyAdvertiserLogic(ctx, svcCtx).GetMyAdvertiser(&pb.GetMyAdvertiserReq{UserId: 1})
	require.NoError(t, err)
	require.True(t, resp.Found)
	require.Equal(t, "Acme", resp.Advertiser.Name)
}

func TestAddQualification(t *testing.T) {
	valid := func() *pb.AddQualificationReq {
		return &pb.AddQualificationReq{
			UserId: 1, Market: " us ", Industry: "financial", DocumentMediaId: 8,
			ValidUntilMs: testNow.Add(24 * 3600e9).UnixMilli(), ExpectedRevision: 2,
		}
	}
	invalid := map[string]func(*pb.AddQualificationReq){
		"market":      func(r *pb.AddQualificationReq) { r.Market = "XX" },
		"industry":    func(r *pb.AddQualificationReq) { r.Industry = "TOYS" },
		"document":    func(r *pb.AddQualificationReq) { r.DocumentMediaId = 0 },
		"expired":     func(r *pb.AddQualificationReq) { r.ValidUntilMs = testNow.UnixMilli() },
		"revision":    func(r *pb.AddQualificationReq) { r.ExpectedRevision = 0 },
		"idempotency": func(r *pb.AddQualificationReq) { r.IdempotencyKey = strings.Repeat("k", 129) },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			svcCtx, _, _ := newSvcCtx(&fakeStore{})
			req := valid()
			mutate(req)
			_, err := NewAddQualificationLogic(context.Background(), svcCtx).AddQualification(req)
			requireCode(t, err, errx.ParamError)
		})
	}

	fs := &fakeStore{advertiser: sampleAdvertiser()}
	svcCtx, _, _ := newSvcCtx(fs)
	resp, err := NewAddQualificationLogic(context.Background(), svcCtx).AddQualification(valid())
	require.NoError(t, err)
	require.True(t, resp.Found)
	require.Equal(t, "US", fs.qualInput.Market)
	require.Equal(t, "FINANCIAL", fs.qualInput.Industry)
	require.Equal(t, int64(8), fs.qualInput.DocumentAssetID)

	svcCtx, _, _ = newSvcCtx(&fakeStore{err: store.ErrQualificationMissing})
	_, err = NewAddQualificationLogic(context.Background(), svcCtx).AddQualification(valid())
	requireCode(t, err, errx.AdQualificationRequired)
}
