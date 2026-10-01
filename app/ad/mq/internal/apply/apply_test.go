package apply

import (
	"context"
	"errors"
	"testing"
	"time"

	"esx/app/ad/internal/serving"
	"esx/app/ad/internal/store"
	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	ad        store.Ad
	snaps     map[int64]*store.Snapshot
	quals     []store.Qualification
	published int64
	effect    store.DecisionEffect
	applied   []event.ReviewDecidedEvent
	closed    []event.ReviewDecidedEvent
	closeErr  error
}

func (f *fakeStore) CloseReportBatch(_ context.Context, d event.ReviewDecidedEvent, _ time.Time) error {
	f.closed = append(f.closed, d)
	return f.closeErr
}

func (f *fakeStore) AdByID(context.Context, int64) (*store.Ad, error) { ad := f.ad; return &ad, nil }
func (f *fakeStore) SnapshotAt(_ context.Context, _ int64, rev int64) (*store.Snapshot, error) {
	if s, ok := f.snaps[rev]; ok {
		return s, nil
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) QualificationsOf(context.Context, int64) ([]store.Qualification, error) {
	return f.quals, nil
}
func (f *fakeStore) GetAsset(_ context.Context, id int64) (*store.Asset, error) {
	return &store.Asset{ID: id, ObjectKey: "assets/1", SHA256: "abc"}, nil
}
func (f *fakeStore) MarkPublished(_ context.Context, _ int64, rev int64) error {
	f.published = rev
	f.ad.PublishedRevision = rev
	return nil
}
func (f *fakeStore) ApplyAdDecision(_ context.Context, d event.ReviewDecidedEvent, _ time.Time) (store.DecisionEffect, error) {
	f.applied = append(f.applied, d)
	return f.effect, nil
}
func (f *fakeStore) ApplyAdvertiserDecision(context.Context, event.ReviewDecidedEvent, time.Time) (store.DecisionEffect, error) {
	return f.effect, nil
}

type fakePublisher struct {
	err   error
	calls int
}

func (f *fakePublisher) Publish(context.Context, string, string) error { f.calls++; return f.err }

type fakeIndex struct {
	upserts map[string]serving.Entry
	removed []string
}

func (f *fakeIndex) Upsert(_ context.Context, market string, e serving.Entry) error {
	if f.upserts == nil {
		f.upserts = map[string]serving.Entry{}
	}
	f.upserts[market] = e
	return nil
}
func (f *fakeIndex) Remove(_ context.Context, market string, _ int64) error {
	f.removed = append(f.removed, market)
	return nil
}

func servingAd(industry string) *fakeStore {
	return &fakeStore{
		ad: store.Ad{ID: 1, AdvertiserID: 9, Revision: 3, ApprovedRevision: 2, ServingStatus: store.ServingServing},
		snaps: map[int64]*store.Snapshot{2: {AdID: 1, Revision: 2, Market: "DE", Industry: industry,
			MediaJSON: `[{"mediaId":5,"sha256":"abc"}]`}},
	}
}

// ADS-011 / ADS-015：投放上一过审快照，发布素材后才进入索引，并从其他市场移除。
func TestRefreshPublishesThenIndexesApprovedSnapshot(t *testing.T) {
	fs, pub, idx := servingAd("GENERAL"), &fakePublisher{}, &fakeIndex{}
	s := &Server{Store: fs, Publisher: pub, Index: idx}
	servable, err := s.Refresh(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, servable)
	require.Equal(t, 1, pub.calls)
	require.Equal(t, int64(2), fs.published)
	require.Equal(t, int64(2), idx.upserts["DE"].Revision)
	require.ElementsMatch(t, []string{"US", "ID"}, idx.removed)

	_, err = s.Refresh(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, pub.calls) // 已发布不重复复制
}

func TestRefreshRemovesWhenNotServable(t *testing.T) {
	ctx := context.Background()
	t.Run("paused", func(t *testing.T) {
		fs, idx := servingAd("GENERAL"), &fakeIndex{}
		fs.ad.ServingStatus = store.ServingOffline
		servable, err := (&Server{Store: fs, Publisher: &fakePublisher{}, Index: idx}).Refresh(ctx, 1)
		require.NoError(t, err)
		require.False(t, servable)
		require.Len(t, idx.removed, 3)
	})
	t.Run("qualification lapsed", func(t *testing.T) {
		fs, idx := servingAd("FINANCIAL"), &fakeIndex{}
		fs.quals = []store.Qualification{{Market: "DE", Industry: "FINANCIAL", Status: store.QualificationExpired, ValidUntilMs: 1}}
		servable, err := (&Server{Store: fs, Publisher: &fakePublisher{}, Index: idx}).Refresh(ctx, 1)
		require.NoError(t, err)
		require.False(t, servable)
		require.Empty(t, idx.upserts)
	})
	t.Run("publish failure keeps ad out of the index", func(t *testing.T) {
		fs, idx := servingAd("GENERAL"), &fakeIndex{}
		_, err := (&Server{Store: fs, Publisher: &fakePublisher{err: errors.New("s3 down")}, Index: idx}).Refresh(ctx, 1)
		require.Error(t, err)
		require.Empty(t, idx.upserts)
		require.Len(t, idx.removed, 3)
	})
}

func TestApplierSkipsStaleRejections(t *testing.T) {
	fs, idx := servingAd("GENERAL"), &fakeIndex{}
	a := &Applier{Store: fs, Server: &Server{Store: fs, Publisher: &fakePublisher{}, Index: idx}}
	stale := event.ReviewDecidedEvent{BizType: event.ReviewBizAdCreative, ObjectID: 1, Revision: 1, TaskID: 1,
		Purpose: event.ReviewPurposeInitial, Verdict: event.ReviewVerdictReject, PolicyCodes: []string{"CONTENT.IP"}}
	require.NoError(t, a.Apply(context.Background(), stale))
	require.Empty(t, idx.upserts)
	fs.effect = store.DecisionEffect{Applied: true, ServingChanged: true}
	approve := stale
	approve.Revision, approve.Verdict, approve.PolicyCodes = 2, event.ReviewVerdictApprove, nil
	require.NoError(t, a.Apply(context.Background(), approve))
	require.Contains(t, idx.upserts, "DE")
}

// ADS-031 / ADS-030：暂停结论与投后任务结论即使未改变状态也刷新索引；举报结论关闭批次。
func TestApplierRefreshesPostServingDecisions(t *testing.T) {
	fs, idx := servingAd("GENERAL"), &fakeIndex{}
	fs.ad.ServingStatus = store.ServingPaused
	a := &Applier{Store: fs, Server: &Server{Store: fs, Publisher: &fakePublisher{}, Index: idx}}
	interim := event.ReviewDecidedEvent{BizType: event.ReviewBizAdCreative, ObjectID: 1, Revision: 2, TaskID: 5,
		Purpose: event.ReviewPurposeRescan, Verdict: event.ReviewVerdictReject, PolicyCodes: []string{"CONTENT.DECEPTIVE"},
		Interim: true}
	require.NoError(t, a.Apply(context.Background(), interim))
	require.Len(t, idx.removed, 3, "暂停后从全部市场移除")
	require.Empty(t, idx.upserts)

	report := interim
	report.Purpose, report.PurposeKey, report.Interim = event.ReviewPurposeReport, "batch-1", false
	fs.closeErr = errors.New("db down")
	require.Error(t, a.Apply(context.Background(), report), "关闭批次失败由消息重试")
	fs.closeErr = nil
	require.NoError(t, a.Apply(context.Background(), report))
	require.Len(t, fs.closed, 3)
	require.Equal(t, "batch-1", fs.closed[2].PurposeKey)
}

type fakeRescanStore struct {
	candidates []store.Ad
	marked     []int64
	submitted  []int64
	skip       map[int64]bool
	err        error
	calls      int
}

func (f *fakeRescanStore) RescanCandidates(_ context.Context, generation string, limit int) ([]store.Ad, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out []store.Ad
	for _, ad := range f.candidates {
		if ad.RescanGeneration != generation && len(out) < limit {
			out = append(out, ad)
		}
	}
	return out, nil
}

func (f *fakeRescanStore) setGeneration(id int64, generation string) {
	for i := range f.candidates {
		if f.candidates[i].ID == id {
			f.candidates[i].RescanGeneration = generation
		}
	}
}

func (f *fakeRescanStore) MarkRescanned(_ context.Context, adID, _ int64, generation string) error {
	f.marked = append(f.marked, adID)
	f.setGeneration(adID, generation)
	return nil
}

func (f *fakeRescanStore) SubmitRescan(_ context.Context, adID, _ int64, generation string, _ time.Time) (bool, error) {
	if f.skip[adID] {
		f.setGeneration(adID, generation)
		return false, nil
	}
	f.submitted = append(f.submitted, adID)
	f.setGeneration(adID, generation)
	return true, nil
}

type fakeGenerations struct {
	gen Generation
	err error
}

func (f fakeGenerations) RescanGeneration(context.Context) (Generation, error) { return f.gen, f.err }

// ADS-031：代次生效前过审的在投广告送回扫，之后过审的只记录代次；代次未就绪时不回扫。
func TestRescannerSubmitsAdsApprovedBeforeTheGeneration(t *testing.T) {
	fs := &fakeRescanStore{candidates: []store.Ad{
		{ID: 1, ApprovedRevision: 1, ApprovedAtMs: 100},
		{ID: 2, ApprovedRevision: 3, ApprovedAtMs: 900},
		{ID: 3, ApprovedRevision: 1, ApprovedAtMs: 200, RescanGeneration: "v2+seeds@1"},
		{ID: 4, ApprovedRevision: 2, ApprovedAtMs: 50},
	}, skip: map[int64]bool{4: true}}
	r := &Rescanner{Store: fs, Source: fakeGenerations{gen: Generation{Ready: false, Value: "v2+seeds@1"}}}
	r.Run(context.Background())
	require.Zero(t, fs.calls)

	r.Source = fakeGenerations{gen: Generation{Ready: true, Value: "v2+seeds@1", EffectiveSinceMs: 500}}
	r.Run(context.Background())
	require.Equal(t, []int64{1}, fs.submitted)
	require.Equal(t, []int64{2}, fs.marked)

	r.Source = fakeGenerations{gen: Generation{Ready: true, Value: "v2+seeds@2", EffectiveSinceMs: 1000}}
	r.Run(context.Background())
	require.Equal(t, []int64{1, 1, 2, 3}, fs.submitted, "新代次重扫全部在投广告；被跳过的 4 不送审")

	fs.err = errors.New("db down")
	r.Run(context.Background())
	r.Source = fakeGenerations{err: errors.New("rpc down")}
	r.Run(context.Background())
}

func TestRescannerPagesThroughLargeGenerations(t *testing.T) {
	fs := &fakeRescanStore{}
	for i := range RescanBatch*2 + 5 {
		fs.candidates = append(fs.candidates, store.Ad{ID: int64(i + 1), ApprovedRevision: 1})
	}
	r := &Rescanner{Store: fs, Source: fakeGenerations{gen: Generation{Ready: true, Value: "g", EffectiveSinceMs: 1}}}
	r.Run(context.Background())
	require.Len(t, fs.submitted, RescanBatch*2+5)
	require.Equal(t, 3, fs.calls)
}
