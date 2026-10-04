package cachedstore

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"esx/pkg/sqlstore"

	"github.com/stretchr/testify/require"
)

func rowKey(id int64) string { return fmt.Sprintf("row:%d", id) }

type batchLoader struct {
	rows  map[int64]cacheRow
	err   error
	calls [][]int64
}

func (b *batchLoader) load(_ context.Context, _ sqlstore.SqlConn, ids []int64) (map[int64]cacheRow, error) {
	b.calls = append(b.calls, append([]int64(nil), ids...))
	if b.err != nil {
		return nil, b.err
	}
	out := make(map[int64]cacheRow, len(ids))
	for _, id := range ids {
		if row, ok := b.rows[id]; ok {
			out[id] = row
		}
	}
	return out, nil
}

func TestQueryRowsByIDsLoadsMissesOnceAndFills(t *testing.T) {
	c, s := newCacheFixture(t)
	ctx := context.Background()
	loader := &batchLoader{rows: map[int64]cacheRow{1: {Status: 1}, 2: {Status: 2}}}

	rows, err := QueryRowsByIDs(ctx, c, []int64{1, 2, 3}, rowKey, loader.load)
	require.NoError(t, err)
	require.Equal(t, map[int64]cacheRow{1: {Status: 1}, 2: {Status: 2}}, rows)
	require.Equal(t, [][]int64{{1, 2, 3}}, loader.calls, "all misses share one SQL load")

	value, ttl := s.Value(Prefix + "row:1")
	require.Contains(t, value, `"Status":1`)
	require.Equal(t, c.options.TTLSeconds, ttl)
	missing, missingTTL := s.Value(Prefix + "row:3")
	require.Contains(t, missing, `"Missing":true`)
	require.Equal(t, c.options.NotFoundTTLSeconds, missingTTL)

	loader.rows[1] = cacheRow{Status: 9}
	rows, err = QueryRowsByIDs(ctx, c, []int64{1, 2, 3}, rowKey, loader.load)
	require.NoError(t, err)
	require.Equal(t, map[int64]cacheRow{1: {Status: 1}, 2: {Status: 2}}, rows, "hits and negative hits skip SQL")
	require.Len(t, loader.calls, 1)
}

func TestQueryRowsByIDsFillFencedByInvalidation(t *testing.T) {
	c, s := newCacheFixture(t)
	ctx := context.Background()
	var rows map[int64]cacheRow
	stale := func(_ context.Context, _ sqlstore.SqlConn, ids []int64) (map[int64]cacheRow, error) {
		// The write commits and invalidates after this read's snapshot.
		require.NoError(t, c.DelCacheCtx(ctx, rowKey(7)))
		return map[int64]cacheRow{7: {Status: 1}}, nil
	}
	rows, err := QueryRowsByIDs(ctx, c, []int64{7}, rowKey, stale)
	require.NoError(t, err)
	require.Equal(t, cacheRow{Status: 1}, rows[7], "the read still returns its own snapshot")
	value, _ := s.Value(Prefix + "row:7")
	require.Empty(t, value, "an invalidated reservation cannot be filled by an older read")
}

func TestQueryRowsByIDsReadsWithoutOwningForeignReservation(t *testing.T) {
	c, s := newCacheFixture(t)
	s.Set(Prefix+"row:7", "fill:other", 30)
	loader := &batchLoader{rows: map[int64]cacheRow{7: {Status: 1}}}
	rows, err := QueryRowsByIDs(context.Background(), c, []int64{7}, rowKey, loader.load)
	require.NoError(t, err)
	require.Equal(t, cacheRow{Status: 1}, rows[7])
	value, _ := s.Value(Prefix + "row:7")
	require.Equal(t, "fill:other", value, "only the reservation owner may fill")
}

func TestQueryRowsByIDsCacheErrorsKeepSQLResult(t *testing.T) {
	for _, command := range []string{"GET", "SET", "EVAL"} {
		t.Run(command, func(t *testing.T) {
			c, s := newCacheFixture(t)
			s.Fail(command, false)
			loader := &batchLoader{rows: map[int64]cacheRow{1: {Status: 1}}}
			rows, err := QueryRowsByIDs(context.Background(), c, []int64{1, 2}, rowKey, loader.load)
			require.NoError(t, err)
			require.Equal(t, map[int64]cacheRow{1: {Status: 1}}, rows)
			require.Len(t, loader.calls, 1)
			value, _ := s.Value(Prefix + "row:1")
			require.NotContains(t, value, `"Status":1`)
		})
	}
}

func TestQueryRowsByIDsFailedLoadReleasesReservations(t *testing.T) {
	c, s := newCacheFixture(t)
	failure := errors.New("SQL unavailable")
	_, err := QueryRowsByIDs(context.Background(), c, []int64{1}, rowKey, (&batchLoader{err: failure}).load)
	require.ErrorIs(t, err, failure)
	value, _ := s.Value(Prefix + "row:1")
	require.Empty(t, value)

	loader := &batchLoader{rows: map[int64]cacheRow{1: {Status: 1}}}
	rows, err := QueryRowsByIDs(context.Background(), c, []int64{1}, rowKey, loader.load)
	require.NoError(t, err)
	require.Equal(t, cacheRow{Status: 1}, rows[1])
	value, _ = s.Value(Prefix + "row:1")
	require.Contains(t, value, `"Status":1`)
}

func TestQueryRowsByIDsWithoutRedisLoadsDirectly(t *testing.T) {
	loader := &batchLoader{rows: map[int64]cacheRow{1: {Status: 1}}}
	rows, err := QueryRowsByIDs(context.Background(), NewConn(nil, nil), []int64{1, 2}, rowKey, loader.load)
	require.NoError(t, err)
	require.Equal(t, map[int64]cacheRow{1: {Status: 1}}, rows)

	empty, err := QueryRowsByIDs(context.Background(), NewConn(nil, nil), nil, rowKey, loader.load)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.Len(t, loader.calls, 1)
}
