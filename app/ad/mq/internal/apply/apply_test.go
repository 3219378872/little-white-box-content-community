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
