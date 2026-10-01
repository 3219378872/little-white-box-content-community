//go:build integration

package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"esx/pkg/event"
	sqlx "esx/pkg/sqlstore"
	"esx/pkg/testutil"
	"esx/pkg/util"

	"github.com/stretchr/testify/require"
)

var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	if err := util.InitSnowflake(22, 1); err != nil {
		panic(err)
	}
	testEnv = testutil.SetupTestEnvM("xbh_ad", testutil.SchemaPath("xbh_ad.sql"))
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	for _, table := range []string{"advertiser", "advertiser_qualification", "ad", "ad_snapshot", "ad_asset", "ad_report", "event_outbox", "idempotency"} {
		_, err := testEnv.DB.Exec("DELETE FROM " + table)
		require.NoError(t, err)
	}
	return New(sqlx.NewSqlConnFromDB(testEnv.DB))
}

func outboxCount(t *testing.T, tag string) int {
	t.Helper()
	var n int
	require.NoError(t, testEnv.DB.QueryRow(`SELECT COUNT(*) FROM event_outbox WHERE topic = 'review-submitted' AND tag = ?`, tag).Scan(&n))
	return n
}

func decided(biz string, objectID, revision int64, verdict string, purpose string, codes ...string) event.ReviewDecidedEvent {
	return event.ReviewDecidedEvent{BizType: biz, ObjectID: objectID, Revision: revision, TaskID: revision, Purpose: purpose,
		Verdict: verdict, PolicyCodes: codes, PolicyVersion: "v1", Source: "human", DecidedAt: 1}
}

// approvedAdvertiser 走真实写路径并应用通过结论。
func approvedAdvertiser(t *testing.T, s *Store, userID int64, now time.Time) *AdvertiserWithQualifications {
	t.Helper()
	ctx := context.Background()
	adv, err := s.ApplyAdvertiser(ctx, AdvertiserInput{UserID: userID, Name: "Acme Coffee", Markets: []string{"US", "DE"}}, now)
	require.NoError(t, err)
	_, err = s.ApplyAdvertiserDecision(ctx, decided(event.ReviewBizAdvertiserQualification, adv.Advertiser.ID, 1,
		event.ReviewVerdictApprove, event.ReviewPurposeInitial), now)
	require.NoError(t, err)
	full, err := s.GetAdvertiserByUser(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "Acme Coffee", full.Advertiser.ApprovedName)
	return full
}

func adInput(market, industry string) AdInput {
	return AdInput{Title: "Fresh beans", Body: "Roasted weekly", CTA: "Shop", LandingURL: "https://coffee.shop.com",
		LandingDomain: "coffee.shop.com", Market: market, Industry: industry}
}

// ADS-001 / ADS-013：广告主送审、幂等与版本冲突；未过审广告主不能投放广告。
func TestAdvertiserLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(1_000_000)
	_, err := s.CreateAd(ctx, 1, adInput("US", "GENERAL"), "", now)
	require.ErrorIs(t, err, ErrAdvertiserNotApproved)

	first, err := s.ApplyAdvertiser(ctx, AdvertiserInput{UserID: 1, Name: "Acme", Markets: []string{"US"}, IdempotencyKey: "k"}, now)
	require.NoError(t, err)
	again, err := s.ApplyAdvertiser(ctx, AdvertiserInput{UserID: 1, Name: "Acme", Markets: []string{"US"}, IdempotencyKey: "k"}, now)
	require.NoError(t, err)
	require.Equal(t, first.Advertiser.ID, again.Advertiser.ID)
	require.Equal(t, 1, outboxCount(t, event.ReviewBizAdvertiserQualification))
	_, err = s.ApplyAdvertiser(ctx, AdvertiserInput{UserID: 1, Name: "Acme 2", Markets: []string{"US"}}, now)
	require.ErrorIs(t, err, ErrAdvertiserExists)
	_, err = s.ApplyAdvertiser(ctx, AdvertiserInput{UserID: 1, Name: "Acme 2", Markets: []string{"US"}, ExpectedRevision: 5}, now)
	require.ErrorIs(t, err, ErrRevisionConflict)

	effect, err := s.ApplyAdvertiserDecision(ctx, decided(event.ReviewBizAdvertiserQualification, first.Advertiser.ID, 1,
		event.ReviewVerdictReject, event.ReviewPurposeInitial, "ACCOUNT.RISK"), now)
	require.NoError(t, err)
	require.True(t, effect.Applied)
	full, err := s.GetAdvertiserByUser(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, ReviewRejected, full.Advertiser.ReviewStatus)
	require.Equal(t, []string{"ACCOUNT.RISK"}, full.Advertiser.PolicyCodes())
}

// ADS-002 / ADS-A08：受监管行业缺少目标市场有效资质时不能送审；资质随广告主 revision 过审后可送审。
func TestRegulatedIndustryRequiresApprovedQualification(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(2_000_000)
	adv := approvedAdvertiser(t, s, 2, now)
	_, err := s.CreateAd(ctx, 2, adInput("DE", "FINANCIAL"), "", now)
	require.ErrorIs(t, err, ErrQualificationMissing)
	_, err = s.CreateAd(ctx, 2, adInput("ID", "GENERAL"), "", now)
	require.ErrorIs(t, err, ErrMarketNotAllowed)

	doc, err := s.CreateAsset(ctx, Asset{ID: 77, UserID: 2, Kind: AssetDocument, SHA256: "d", MimeType: "application/pdf", SizeBytes: 1, ObjectKey: "assets/77"}, "", now)
	require.NoError(t, err)
	_, err = s.AddQualification(ctx, QualificationInput{UserID: 2, Market: "DE", Industry: "FINANCIAL", DocumentAssetID: doc.ID,
		ValidUntilMs: now.Add(time.Hour).UnixMilli(), ExpectedRevision: 1}, now)
	require.NoError(t, err)
	_, err = s.ApplyAdvertiserDecision(ctx, decided(event.ReviewBizAdvertiserQualification, adv.Advertiser.ID, 2,
		event.ReviewVerdictApprove, event.ReviewPurposeInitial), now)
	require.NoError(t, err)
	created, err := s.CreateAd(ctx, 2, adInput("DE", "FINANCIAL"), "", now)
	require.NoError(t, err)
	require.Equal(t, ReviewPending, created.Ad.ReviewStatus)

	// 酒精允许送审，由审核平台硬规则拒绝（ADS-017）。
	_, err = s.CreateAd(ctx, 2, adInput("US", "ALCOHOL"), "", now)
	require.NoError(t, err)

	expired, err := s.ExpireQualifications(ctx, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, expired, 1)
	ids, err := s.MarkQualificationLapsed(ctx, expired[0], now)
	require.NoError(t, err)
	require.Contains(t, ids, created.Ad.ID)
}

// ADS-011 / RVW-040 / ADS-A02：编辑过审广告后旧快照继续投放；乱序旧结论不生效；新版本通过后切换。
func TestEditApprovedAdKeepsServingOldSnapshot(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(3_000_000)
	approvedAdvertiser(t, s, 3, now)
	created, err := s.CreateAd(ctx, 3, adInput("US", "GENERAL"), "create-1", now)
	require.NoError(t, err)
	replay, err := s.CreateAd(ctx, 3, adInput("US", "GENERAL"), "create-1", now)
	require.NoError(t, err)
	require.Equal(t, created.Ad.ID, replay.Ad.ID)
	adID := created.Ad.ID

	effect, err := s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictApprove, event.ReviewPurposeInitial), now)
	require.NoError(t, err)
	require.True(t, effect.ServingChanged)

	edit := adInput("US", "GENERAL")
	edit.Title = "New beans"
	updated, err := s.UpdateAd(ctx, 3, adID, 1, edit, "", now)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.Ad.Revision)
	require.Equal(t, int64(1), updated.Ad.ApprovedRevision)
	require.Equal(t, "Fresh beans", updated.Approved.Title)
	require.Equal(t, ServingServing, updated.Ad.ServingStatus)
	_, err = s.UpdateAd(ctx, 3, adID, 1, edit, "", now)
	require.ErrorIs(t, err, ErrRevisionConflict)
	_, err = s.GetAd(ctx, 999, adID)
	require.ErrorIs(t, err, ErrNotFound)

	// 新版本被拒：仍投旧快照。
	_, err = s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 2, event.ReviewVerdictReject, event.ReviewPurposeInitial, "MISLEADING.CLAIM"), now)
	require.NoError(t, err)
	ad, err := s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ReviewRejected, ad.ReviewStatus)
	require.Equal(t, int64(1), ad.ApprovedRevision)

	// 再编辑并通过：切换到新快照；随后到达的旧 revision 结论不生效。
	_, err = s.UpdateAd(ctx, 3, adID, 2, edit, "", now)
	require.NoError(t, err)
	_, err = s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 3, event.ReviewVerdictApprove, event.ReviewPurposeInitial), now)
	require.NoError(t, err)
	stale, err := s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 2, event.ReviewVerdictApprove, event.ReviewPurposeInitial), now)
	require.NoError(t, err)
	require.False(t, stale.Applied)
	full, err := s.GetAd(ctx, 3, adID)
	require.NoError(t, err)
	require.Equal(t, int64(3), full.Ad.ApprovedRevision)
	require.Equal(t, "New beans", full.Approved.Title)
	require.Equal(t, 3, outboxCount(t, event.ReviewBizAdCreative))

	// 质检判定违规：立即下线（RVW-014）。
	qa, err := s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 3, event.ReviewVerdictReject, event.ReviewPurposeQA, "MISLEADING.CLAIM"), now)
	require.NoError(t, err)
	require.True(t, qa.ServingChanged)
	ad, err = s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingOffline, ad.ServingStatus)
}

// RVW-041：送审超过 5 分钟仍无任务的广告被找出，记录任务后不再出现。
func TestReconcileFindsStalePendingAds(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(4_000_000)
	approvedAdvertiser(t, s, 4, now)
	created, err := s.CreateAd(ctx, 4, adInput("US", "GENERAL"), "", now)
	require.NoError(t, err)
	pending, err := s.PendingAdsWithoutTask(ctx, now.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Empty(t, pending)
	pending, err = s.PendingAdsWithoutTask(ctx, now.Add(6*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	snap, err := s.SnapshotAt(ctx, created.Ad.ID, 1)
	require.NoError(t, err)
	sub := AdSubmission(&pending[0], snap, event.ReviewPurposeInitial, now)
	require.NoError(t, sub.Validate())
	require.Equal(t, "Acme Coffee", sub.Snapshot.Texts["advertiser"])
	require.NoError(t, s.RecordAdTask(ctx, created.Ad.ID, 1, 555))
	pending, err = s.PendingAdsWithoutTask(ctx, now.Add(6*time.Minute), 10)
	require.NoError(t, err)
	require.Empty(t, pending)
}

// servingAd 创建并通过一条广告，返回其 ID。
func servingAd(t *testing.T, s *Store, userID int64, now time.Time) int64 {
	t.Helper()
	approvedAdvertiser(t, s, userID, now)
	created, err := s.CreateAd(context.Background(), userID, adInput("US", "GENERAL"), "", now)
	require.NoError(t, err)
	approve := decided(event.ReviewBizAdCreative, created.Ad.ID, 1, event.ReviewVerdictApprove, event.ReviewPurposeInitial)
	approve.DecidedAt = now.UnixMilli()
	_, err = s.ApplyAdDecision(context.Background(), approve, now)
	require.NoError(t, err)
	return created.Ad.ID
}

// submissions 按写入顺序返回某目的的送审事件（outbox 载荷是 LONGBLOB，在 Go 侧解码过滤）。
func submissions(t *testing.T, purpose string) []event.ReviewSubmittedEvent {
	t.Helper()
	rows, err := testEnv.DB.Query(`SELECT payload FROM event_outbox WHERE topic = 'review-submitted' ORDER BY created_at, id`)
	require.NoError(t, err)
	defer rows.Close()
	var out []event.ReviewSubmittedEvent
	for rows.Next() {
		var payload []byte
		require.NoError(t, rows.Scan(&payload))
		var sub event.ReviewSubmittedEvent
		require.NoError(t, json.Unmarshal(payload, &sub))
		if sub.Purpose == purpose {
			require.NoError(t, sub.Validate())
			out = append(out, sub)
		}
	}
	require.NoError(t, rows.Err())
	return out
}

func lastSubmission(t *testing.T, purpose string) event.ReviewSubmittedEvent {
	t.Helper()
	subs := submissions(t, purpose)
	require.NotEmpty(t, subs)
	return subs[len(subs)-1]
}

func submissionCount(t *testing.T, purpose string) int {
	t.Helper()
	return len(submissions(t, purpose))
}

// ADS-031 / ADS-A07：回扫暂停结论先停投；人审确认下线，否定恢复；更新的 revision 过审也解除暂停。
func TestRescanPauseThenResumeOrOffline(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(5_000_000)
	adID := servingAd(t, s, 5, now)
	interim := decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictReject, event.ReviewPurposeRescan, "CONTENT.DECEPTIVE")
	interim.Interim = true
	effect, err := s.ApplyAdDecision(ctx, interim, now)
	require.NoError(t, err)
	require.True(t, effect.ServingChanged)
	ad, err := s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingPaused, ad.ServingStatus)
	require.Equal(t, PauseRescan, ad.PauseReason)
	require.Equal(t, []string{"CONTENT.DECEPTIVE"}, ad.PolicyCodes())

	// 暂停期间资质失效不覆盖暂停原因。
	_, err = s.MarkQualificationLapsed(ctx, Qualification{AdvertiserID: ad.AdvertiserID, Market: "US", Industry: "GENERAL"}, now)
	require.NoError(t, err)
	resume := decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictApprove, event.ReviewPurposeRescan)
	effect, err = s.ApplyAdDecision(ctx, resume, now)
	require.NoError(t, err)
	require.True(t, effect.ServingChanged)
	ad, err = s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingServing, ad.ServingStatus)
	require.Empty(t, ad.PauseReason)
	require.Empty(t, ad.PolicyCodes())
	again, err := s.ApplyAdDecision(ctx, resume, now)
	require.NoError(t, err)
	require.False(t, again.Applied, "重复投递无副作用")

	// 再次暂停后，编辑出的新 revision 过审：投放新快照并解除暂停。
	_, err = s.ApplyAdDecision(ctx, interim, now)
	require.NoError(t, err)
	edit := adInput("US", "GENERAL")
	edit.Title = "Honest beans"
	_, err = s.UpdateAd(ctx, 5, adID, 1, edit, "", now)
	require.NoError(t, err)
	_, err = s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 2, event.ReviewVerdictApprove, event.ReviewPurposeInitial), now)
	require.NoError(t, err)
	ad, err = s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingServing, ad.ServingStatus)
	require.Equal(t, int64(2), ad.ApprovedRevision)
	stale, err := s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictReject,
		event.ReviewPurposeRescan, "CONTENT.DECEPTIVE"), now)
	require.NoError(t, err)
	require.False(t, stale.Applied, "旧快照的回扫结论不再改变状态")

	// 人审确认违规：下线并以政策码告知。
	interim.Revision = 2
	_, err = s.ApplyAdDecision(ctx, interim, now)
	require.NoError(t, err)
	offline, err := s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 2, event.ReviewVerdictReject,
		event.ReviewPurposeRescan, "CONTENT.DECEPTIVE"), now)
	require.NoError(t, err)
	require.True(t, offline.ServingChanged)
	ad, err = s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingOffline, ad.ServingStatus)
	require.Equal(t, PauseRescan, ad.PauseReason)
	require.Equal(t, ReviewRejected, ad.ReviewStatus)
}

// ADS-014：被下线或被拒的 revision 各可申诉一次；申诉通过恢复投放，拒绝维持原状。
func TestAppealOncePerRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(6_000_000)
	adID := servingAd(t, s, 6, now)
	_, err := s.AppealAd(ctx, 6, adID, "", now)
	require.ErrorIs(t, err, ErrAppealNotAllowed, "在投广告不可申诉")
	_, err = s.AppealAd(ctx, 999, adID, "", now)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictReject,
		event.ReviewPurposeReport, "CONTENT.DECEPTIVE"), now)
	require.NoError(t, err)
	appealed, err := s.AppealAd(ctx, 6, adID, "appeal-1", now)
	require.NoError(t, err)
	require.Equal(t, ReviewAppealing, appealed.Ad.ReviewStatus)
	require.Equal(t, int64(1), appealed.Ad.AppealedRevision)
	replay, err := s.AppealAd(ctx, 6, adID, "appeal-1", now)
	require.NoError(t, err)
	require.Equal(t, ReviewAppealing, replay.Ad.ReviewStatus)
	require.Equal(t, 1, submissionCount(t, event.ReviewPurposeAppeal))
	sub := lastSubmission(t, event.ReviewPurposeAppeal)
	require.Equal(t, int64(1), sub.Revision)
	require.Equal(t, "Fresh beans", sub.Snapshot.Texts["title"])
	pending, err := s.PendingAdsWithoutTask(ctx, now.Add(6*time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, pending, 1, "申诉同样受对账保护")

	approve := decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictApprove, event.ReviewPurposeAppeal)
	effect, err := s.ApplyAdDecision(ctx, approve, now)
	require.NoError(t, err)
	require.True(t, effect.ServingChanged)
	ad, err := s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingServing, ad.ServingStatus)
	require.Equal(t, ReviewApproved, ad.ReviewStatus)
	require.Empty(t, ad.PauseReason)
	_, ok := AppealTarget(ad)
	require.False(t, ok)

	// 新 revision 被拒：可申诉一次，申诉被拒后不可再申诉。
	_, err = s.UpdateAd(ctx, 6, adID, 1, adInput("US", "GENERAL"), "", now)
	require.NoError(t, err)
	_, err = s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 2, event.ReviewVerdictReject,
		event.ReviewPurposeInitial, "MISLEADING.CLAIM"), now)
	require.NoError(t, err)
	_, err = s.AppealAd(ctx, 6, adID, "", now)
	require.NoError(t, err)
	rejected, err := s.ApplyAdDecision(ctx, decided(event.ReviewBizAdCreative, adID, 2, event.ReviewVerdictReject,
		event.ReviewPurposeAppeal, "MISLEADING.CLAIM"), now)
	require.NoError(t, err)
	require.True(t, rejected.Applied)
	_, err = s.AppealAd(ctx, 6, adID, "", now)
	require.ErrorIs(t, err, ErrAppealNotAllowed)
	ad, err = s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ReviewRejected, ad.ReviewStatus)
	require.Equal(t, int64(1), ad.ApprovedRevision, "旧过审快照继续投放")
	require.Equal(t, ServingServing, ad.ServingStatus)
}

// ADS-030：同一身份只计一次；批次内举报共用任务并提高优先级；举报成立下线；结论后到达的举报进入新批次。
func TestReportBatchesAndCarryOver(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(7_000_000)
	adID := servingAd(t, s, 7, now)
	_, err := s.ReportAd(ctx, ReportInput{AdID: 424242, ReporterKey: "u:1", Reason: "scam"}, now)
	require.ErrorIs(t, err, ErrNotFound)

	first, err := s.ReportAd(ctx, ReportInput{AdID: adID, ReporterKey: "u:1", Reason: "scam"}, now)
	require.NoError(t, err)
	require.Equal(t, ReportResult{Counted: true, Count: 1}, first)
	dup, err := s.ReportAd(ctx, ReportInput{AdID: adID, ReporterKey: "u:1", Reason: "other"}, now)
	require.NoError(t, err)
	require.False(t, dup.Counted)
	second, err := s.ReportAd(ctx, ReportInput{AdID: adID, ReporterKey: "s:abc", Reason: "misleading"}, now)
	require.NoError(t, err)
	require.Equal(t, 2, second.Count)
	sub := lastSubmission(t, event.ReviewPurposeReport)
	require.Equal(t, int64(1), sub.Revision)
	require.Equal(t, ReportPriority(2), sub.Priority)
	batch := sub.PurposeKey
	require.NotEmpty(t, batch)
	require.Equal(t, 2, submissionCount(t, event.ReviewPurposeReport))

	// 举报未成立，但结论作出后又到达一条举报：移入新批次重新送审。
	decidedAt := now.Add(time.Minute)
	_, err = s.ReportAd(ctx, ReportInput{AdID: adID, ReporterKey: "u:3", Reason: "scam"}, decidedAt.Add(time.Second))
	require.NoError(t, err)
	clear := decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictApprove, event.ReviewPurposeReport)
	clear.PurposeKey, clear.DecidedAt = batch, decidedAt.UnixMilli()
	require.NoError(t, s.CloseReportBatch(ctx, clear, decidedAt.Add(2*time.Second)))
	require.NoError(t, s.CloseReportBatch(ctx, clear, decidedAt.Add(2*time.Second)), "重复投递无副作用")
	carried := lastSubmission(t, event.ReviewPurposeReport)
	require.NotEqual(t, batch, carried.PurposeKey)
	require.Equal(t, ReportPriority(1), carried.Priority)
	require.Equal(t, 4, submissionCount(t, event.ReviewPurposeReport))
	ad, err := s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, carried.PurposeKey, ad.ReportBatch)

	// 举报成立：下线，关闭批次；下线后的举报不再计数。
	upheld := decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictReject, event.ReviewPurposeReport, "CONTENT.DECEPTIVE")
	upheld.PurposeKey, upheld.DecidedAt = carried.PurposeKey, decidedAt.Add(time.Hour).UnixMilli()
	effect, err := s.ApplyAdDecision(ctx, upheld, now)
	require.NoError(t, err)
	require.True(t, effect.ServingChanged)
	require.NoError(t, s.CloseReportBatch(ctx, upheld, now))
	ad, err = s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, ServingOffline, ad.ServingStatus)
	require.Equal(t, PauseReport, ad.PauseReason)
	require.Empty(t, ad.ReportBatch)
	late, err := s.ReportAd(ctx, ReportInput{AdID: adID, ReporterKey: "u:4", Reason: "scam"}, now)
	require.NoError(t, err)
	require.False(t, late.Counted)
}

// ADS-031：只回扫在投广告；按代次与过审 revision 幂等。
func TestRescanCandidatesAndSubmission(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.UnixMilli(8_000_000)
	adID := servingAd(t, s, 8, now)
	ad, err := s.AdByID(ctx, adID)
	require.NoError(t, err)
	require.Equal(t, now.UnixMilli(), ad.ApprovedAtMs)

	candidates, err := s.RescanCandidates(ctx, "v2+seeds@0", 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	ok, err := s.SubmitRescan(ctx, adID, 99, "v2+seeds@0", now)
	require.NoError(t, err)
	require.False(t, ok, "过审 revision 已变化时跳过")
	ok, err = s.SubmitRescan(ctx, adID, 1, "v2+seeds@0", now)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = s.SubmitRescan(ctx, adID, 1, "v2+seeds@0", now)
	require.NoError(t, err)
	require.False(t, ok)
	sub := lastSubmission(t, event.ReviewPurposeRescan)
	require.Equal(t, "v2+seeds@0", sub.PurposeKey)
	require.Equal(t, int64(1), sub.Revision)
	candidates, err = s.RescanCandidates(ctx, "v2+seeds@0", 10)
	require.NoError(t, err)
	require.Empty(t, candidates)

	require.NoError(t, s.MarkRescanned(ctx, adID, 1, "v3+seeds@0"))
	candidates, err = s.RescanCandidates(ctx, "v3+seeds@0", 10)
	require.NoError(t, err)
	require.Empty(t, candidates)
	interim := decided(event.ReviewBizAdCreative, adID, 1, event.ReviewVerdictReject, event.ReviewPurposeRescan, "CONTENT.DECEPTIVE")
	interim.Interim = true
	_, err = s.ApplyAdDecision(ctx, interim, now)
	require.NoError(t, err)
	candidates, err = s.RescanCandidates(ctx, "v4+seeds@0", 10)
	require.NoError(t, err)
	require.Empty(t, candidates, "暂停中的广告等待人审，不重复回扫")
}
