package logic

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	pb "esx/kitex_gen/ad"
	"esx/pkg/errx"
)

// ADS-030 / ADS-026：举报按身份去重计数并对举报人隐藏；原因必须是受支持的结构化原因。
func TestReportAd(t *testing.T) {
	fs := &fakeStore{report: store.ReportResult{Counted: true, Count: 2}}
	svcCtx, _, kv := newSvcCtx(fs)
	l := NewReportAdLogic(context.Background(), svcCtx)

	for _, req := range []*pb.ReportAdReq{
		{AdId: 7, Reason: "scam"},
		{UserId: 1, Reason: "scam"},
		{UserId: 1, AdId: 7, Reason: "boring"},
		{UserId: 1, AdId: 7},
	} {
		_, err := l.ReportAd(req)
		requireCode(t, err, errx.ParamError)
	}
	require.Zero(t, fs.calls["ReportAd"])

	resp, err := l.ReportAd(&pb.ReportAdReq{SessionId: "anon-1", AdId: 7, Reason: " scam "})
	require.NoError(t, err)
	require.True(t, resp.GetCounted())
	identity, _ := serving.Identity(0, "anon-1")
	require.Equal(t, store.ReportInput{AdID: 7, ReporterKey: identity, Reason: "scam"}, fs.reportInput)
	require.Contains(t, kv.hashes[serving.HiddenKey(identity)], "7")

	fs.report = store.ReportResult{}
	resp, err = l.ReportAd(&pb.ReportAdReq{UserId: 1, AdId: 7, Reason: "other"})
	require.NoError(t, err)
	require.False(t, resp.GetCounted())
	require.Equal(t, "u:1", fs.reportInput.ReporterKey)
}

func TestReportAdFailures(t *testing.T) {
	fs := &fakeStore{err: store.ErrNotFound}
	svcCtx, _, kv := newSvcCtx(fs)
	_, err := NewReportAdLogic(context.Background(), svcCtx).ReportAd(&pb.ReportAdReq{UserId: 1, AdId: 7, Reason: "scam"})
	requireCode(t, err, errx.NotFound)

	fs.err = nil
	kv.evalErr = errBoom
	_, err = NewReportAdLogic(context.Background(), svcCtx).ReportAd(&pb.ReportAdReq{UserId: 1, AdId: 7, Reason: "scam"})
	requireCode(t, err, errx.SystemError)
	require.Equal(t, 2, fs.calls["ReportAd"], "举报先持久化，隐藏失败由客户端重试且举报去重")
}

// ADS-014：申诉需要登录；不可申诉返回 7106；可申诉状态出现在广告视图中。
func TestAppealAd(t *testing.T) {
	fs := &fakeStore{ad: &store.AdWithSnapshots{
		Ad: &store.Ad{ID: 7, AdvertiserID: 40, Revision: 2, ApprovedRevision: 1, ReviewStatus: store.ReviewAppealing,
			ServingStatus: store.ServingServing, AppealedRevision: 2, PolicyCodesJSON: "[]"},
		Latest: snapshotFor(7),
	}}
	svcCtx, _, _ := newSvcCtx(fs)
	l := NewAppealAdLogic(context.Background(), svcCtx)

	_, err := l.AppealAd(&pb.AppealAdReq{AdId: 7})
	requireCode(t, err, errx.LoginRequired)
	_, err = l.AppealAd(&pb.AppealAdReq{UserId: 1})
	requireCode(t, err, errx.ParamError)

	resp, err := l.AppealAd(&pb.AppealAdReq{UserId: 1, AdId: 7, IdempotencyKey: "appeal-1"})
	require.NoError(t, err)
	require.Equal(t, "appealing", resp.GetAd().GetReviewStatus())
	require.Equal(t, int64(2), resp.GetAd().GetAppealedRevision())
	require.False(t, resp.GetAd().GetAppealable())
	require.Equal(t, "appeal-1", fs.appealKey)

	fs.err = store.ErrAppealNotAllowed
	_, err = l.AppealAd(&pb.AppealAdReq{UserId: 1, AdId: 7})
	requireCode(t, err, errx.AdAppealNotAllowed)
}

func TestAppealTarget(t *testing.T) {
	cases := map[string]struct {
		ad     store.Ad
		target int64
		ok     bool
	}{
		"rejected latest":   {store.Ad{Revision: 3, ApprovedRevision: 2, ReviewStatus: store.ReviewRejected}, 3, true},
		"already appealed":  {store.Ad{Revision: 3, ReviewStatus: store.ReviewRejected, AppealedRevision: 3}, 0, false},
		"offline approved":  {store.Ad{Revision: 2, ApprovedRevision: 2, ReviewStatus: store.ReviewApproved, ServingStatus: store.ServingOffline}, 2, true},
		"offline with edit": {store.Ad{Revision: 3, ApprovedRevision: 2, ReviewStatus: store.ReviewPending, ServingStatus: store.ServingOffline}, 0, false},
		"serving":           {store.Ad{Revision: 2, ApprovedRevision: 2, ReviewStatus: store.ReviewApproved, ServingStatus: store.ServingServing}, 0, false},
		"appealing":         {store.Ad{Revision: 2, ReviewStatus: store.ReviewAppealing, AppealedRevision: 2}, 0, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			target, ok := store.AppealTarget(&tc.ad)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.target, target)
		})
	}
}
