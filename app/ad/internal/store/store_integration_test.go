//go:build integration

package store

import (
	"context"
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
	for _, table := range []string{"advertiser", "advertiser_qualification", "ad", "ad_snapshot", "ad_asset", "event_outbox", "idempotency"} {
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
