package router

import (
	"context"
	"errors"
	"testing"

	"esx/app/review/internal/cascade"
	"esx/app/review/internal/store"

	"github.com/stretchr/testify/require"
)

type fakeEmbedder struct{ err error }

func (f fakeEmbedder) Embed(context.Context, string) ([]float32, string, error) {
	return []float32{3, 4}, "minilm@1", f.err
}

type fakeIndex struct {
	hits    []Hit
	err     error
	vector  []float32
	market  string
	inserts []int64
	deletes []int64
}

func (f *fakeIndex) Search(_ context.Context, vector []float32, market string, _ int) ([]Hit, error) {
	f.vector, f.market = vector, market
	return f.hits, f.err
}

func (f *fakeIndex) Insert(_ context.Context, id int64, _, _ string, _ []float32) error {
	f.inserts = append(f.inserts, id)
	return nil
}

func (f *fakeIndex) Delete(_ context.Context, ids []int64) error {
	f.deletes = append(f.deletes, ids...)
	return nil
}

type fakeAuthority struct {
	statuses map[int64]string
	count    int64
	seeds    []store.Seed
	marked   map[int64]string
}

func (f *fakeAuthority) SeedStatuses(context.Context, []int64) (map[int64]string, error) {
	return f.statuses, nil
}

func (f *fakeAuthority) ActiveSeedSummary(context.Context, string) (int64, int64, error) {
	return f.count, 42, nil
}

func (f *fakeAuthority) SeedsToSync(context.Context, int) ([]store.Seed, error) { return f.seeds, nil }

func (f *fakeAuthority) MarkSeedIndex(_ context.Context, id int64, _, state string) error {
	if f.marked == nil {
		f.marked = map[int64]string{}
	}
	f.marked[id] = state
	return nil
}

func TestRouteUsesAuthoritativeSeedStatus(t *testing.T) {
	index := &fakeIndex{hits: []Hit{
		{SeedID: 1, Issue: "MISLEADING.CLAIM", Score: 0.91},
		{SeedID: 2, Issue: "MISLEADING.CLAIM", Score: 0.97},
		{SeedID: 3, Issue: "CONTENT.VIOLENCE", Score: 0.5},
	}}
	authority := &fakeAuthority{count: 3, statuses: map[int64]string{1: "active", 2: "retired", 3: "active"}}
	r := &Router{Embedder: fakeEmbedder{}, Index: index, Authority: authority}
	result, err := r.Route(context.Background(), cascade.RouteInput{Market: "US", Text: "x"})
	require.NoError(t, err)
	require.True(t, result.Covered)
	require.InDelta(t, 0.91, result.Similarity["MISLEADING.CLAIM"], 1e-9) // 停用种子 2 不计入
	require.Equal(t, []int64{1}, result.SeedIDs["MISLEADING.CLAIM"])
	require.Equal(t, "US", index.market)
	require.InDelta(t, 0.6, index.vector[0], 1e-6) // 归一化
	require.Equal(t, "seedbank@3.42+minilm@1", r.Version())
}

func TestRouteReportsNoCoverageAndFailures(t *testing.T) {
	r := &Router{Embedder: fakeEmbedder{}, Index: &fakeIndex{}, Authority: &fakeAuthority{}}
	result, err := r.Route(context.Background(), cascade.RouteInput{Market: "ID"})
	require.NoError(t, err)
	require.False(t, result.Covered)

	r = &Router{Embedder: fakeEmbedder{}, Index: &fakeIndex{err: errors.New("down")}, Authority: &fakeAuthority{count: 1}}
	_, err = r.Route(context.Background(), cascade.RouteInput{Market: "US"})
	require.ErrorIs(t, err, cascade.ErrUnavailable)
	_, err = (&Router{}).Route(context.Background(), cascade.RouteInput{})
	require.ErrorIs(t, err, cascade.ErrUnavailable)
}

func TestSyncSeedsInsertsActiveAndRemovesRetired(t *testing.T) {
	authority := &fakeAuthority{seeds: []store.Seed{
		{ID: 1, Status: store.SeedActive, Market: "US", IssueCode: "A", Text: "t"},
		{ID: 2, Status: store.SeedRetired},
	}}
	index := &fakeIndex{}
	n, err := SyncSeeds(context.Background(), authority, fakeEmbedder{}, index, 10)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []int64{1}, index.inserts)
	require.Equal(t, []int64{2}, index.deletes)
	require.Equal(t, store.SeedIndexed, authority.marked[1])
	require.Equal(t, store.SeedRemoved, authority.marked[2])
	require.Equal(t, "MARKET", sanitizeMarket(`MARKET" || x == "1`))
}
