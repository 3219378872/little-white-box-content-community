package cachedstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"esx/pkg/modelcache"
	"esx/pkg/redisstore"
	"esx/pkg/sqlstore"
	"esx/pkg/testutil/redisfixture"

	"github.com/stretchr/testify/require"
)

type cacheRow struct{ Status int }

func newCacheFixture(t *testing.T) (CachedConn, *redisfixture.Server) {
	t.Helper()
	s := redisfixture.New(t)
	c := NewConn(nil, modelcache.CacheConf{{RedisConf: redisstore.RedisConf{Host: s.Addr()}}})
	t.Cleanup(func() { require.NoError(t, c.redis.Close()) })
	return c, s
}

func rowQuery(value int, err error) func(context.Context, sqlstore.SqlConn, any) error {
	return func(_ context.Context, _ sqlstore.SqlConn, out any) error {
		*out.(*cacheRow) = cacheRow{Status: value}
		return err
	}
}

func TestCacheFillFencedByInvalidation(t *testing.T) {
	for _, tc := range []struct {
		name                string
		before, after       int
		beforeErr, afterErr error
	}{
		{name: "update", before: 1, after: 2},
		{name: "soft delete", before: 1, after: 0},
		{name: "hard delete", before: 1, afterErr: ErrNotFound},
		{name: "create after missing", beforeErr: ErrNotFound, after: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, s := newCacheFixture(t)
			ctx := context.Background()
			loaded, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() {
				var old cacheRow
				done <- c.QueryRowCtx(ctx, &old, "row:7", func(_ context.Context, _ sqlstore.SqlConn, out any) error {
					*out.(*cacheRow) = cacheRow{Status: tc.before}
					close(loaded)
					<-release
					return tc.beforeErr
				})
			}()
			<-loaded
			marker, ttl := s.Value(Prefix + "row:7")
			// The authoritative write has committed before invalidation.
			err := c.DelCacheCtx(ctx, "row:7")
			close(release)
			require.NoError(t, err)
			require.ErrorIs(t, <-done, tc.beforeErr)
			value, _ := s.Value(Prefix + "row:7")
			require.Empty(t, value, "stale result must not repopulate invalidated key")
			require.Contains(t, marker, "fill:")
			require.Equal(t, fillReservationTTLSeconds, ttl)
			var fresh cacheRow
			calls := 0
			query := func(ctx context.Context, conn sqlstore.SqlConn, out any) error {
				calls++
				return rowQuery(tc.after, tc.afterErr)(ctx, conn, out)
			}
			require.ErrorIs(t, c.QueryRowCtx(ctx, &fresh, "row:7", query), tc.afterErr)
			require.Equal(t, tc.after, fresh.Status)
			require.ErrorIs(t, c.QueryRowCtx(ctx, &fresh, "row:7", query), tc.afterErr)
			require.Equal(t, 1, calls, "current result should subsequently hit cache")
			_, ttl = s.Value(Prefix + "row:7")
			wantTTL := c.options.TTLSeconds
			if tc.afterErr != nil {
				wantTTL = c.options.NotFoundTTLSeconds
			}
			require.Equal(t, wantTTL, ttl)
		})
	}
}

func TestOldReservationCannotReplaceOrReleaseNewOwner(t *testing.T) {
	for _, expire := range []bool{false, true} {
		for _, oldErr := range []error{nil, errors.New("SQL failed")} {
			t.Run(map[bool]string{false: "invalidate", true: "expire"}[expire]+"/"+errorName(oldErr), func(t *testing.T) {
				c, s := newCacheFixture(t)
				ctx := context.Background()
				loaded, release := make(chan struct{}), make(chan struct{})
				done := make(chan error, 1)
				go func() {
					var row cacheRow
					done <- c.QueryRowCtx(ctx, &row, "row:7", func(_ context.Context, _ sqlstore.SqlConn, out any) error {
						*out.(*cacheRow) = cacheRow{Status: 1}
						close(loaded)
						<-release
						return oldErr
					})
				}()
				<-loaded
				if expire {
					s.Expire(Prefix + "row:7")
				} else {
					require.NoError(t, c.DelCacheCtx(ctx, "row:7"))
				}
				var current cacheRow
				require.NoError(t, c.QueryRowCtx(ctx, &current, "row:7", func(_ context.Context, _ sqlstore.SqlConn, out any) error {
					marker, _ := s.Value(Prefix + "row:7")
					close(release)
					require.ErrorIs(t, <-done, oldErr)
					still, _ := s.Value(Prefix + "row:7")
					require.Equal(t, marker, still, "old owner must not overwrite or release the new reservation")
					*out.(*cacheRow) = cacheRow{Status: 2}
					return nil
				}))
				require.NoError(t, c.QueryRowCtx(ctx, &current, "row:7", rowQuery(99, errors.New("unexpected SQL"))))
				require.Equal(t, 2, current.Status)
			})
		}
	}
}

func errorName(err error) string {
	if err == nil {
		return "success"
	}
	return "error"
}

func TestConcurrentMissWithoutReservationDoesNotFill(t *testing.T) {
	c, s := newCacheFixture(t)
	ctx := context.Background()
	var owner cacheRow
	require.NoError(t, c.QueryRowCtx(ctx, &owner, "row:7", func(_ context.Context, _ sqlstore.SqlConn, out any) error {
		marker, _ := s.Value(Prefix + "row:7")
		var concurrent cacheRow
		require.NoError(t, c.QueryRowCtx(ctx, &concurrent, "row:7", rowQuery(2, nil)))
		require.Equal(t, 2, concurrent.Status)
		still, _ := s.Value(Prefix + "row:7")
		require.Equal(t, marker, still)
		*out.(*cacheRow) = cacheRow{Status: 1}
		return nil
	}))
}

func TestIndexMissAlwaysLoadsFreshPrimary(t *testing.T) {
	c, _ := newCacheFixture(t)
	ctx := context.Background()
	var row cacheRow
	primaryCalls := 0
	err := c.QueryRowIndexCtx(ctx, &row, "index:old", func(any) string { return "row:7" },
		func(ctx context.Context, _ sqlstore.SqlConn, out any) (any, error) {
			*out.(*cacheRow) = cacheRow{Status: 1}
			// A primary update commits after the index SELECT took its snapshot.
			return 7, c.DelCacheCtx(ctx, "row:7")
		}, func(_ context.Context, _ sqlstore.SqlConn, out, id any) error {
			primaryCalls++
			require.Equal(t, "7", id)
			*out.(*cacheRow) = cacheRow{Status: 0}
			return nil
		})
	require.NoError(t, err)
	require.Equal(t, 1, primaryCalls)
	require.Zero(t, row.Status)
	require.NoError(t, c.QueryRowCtx(ctx, &row, "row:7", rowQuery(99, errors.New("unexpected SQL"))))
	require.Zero(t, row.Status)
}

func TestLegacyCacheNamespaceIsNotRead(t *testing.T) {
	c, s := newCacheFixture(t)
	s.Set("cache:v2:row:7", `{"Missing":false,"Value":{"Status":1}}`, 604800)
	var row cacheRow
	require.NoError(t, c.QueryRowCtx(context.Background(), &row, "row:7", rowQuery(0, nil)))
	require.Zero(t, row.Status)
}

func TestInvalidationUsesSingleKeysAndCommitRemainsSuccessful(t *testing.T) {
	c, s := newCacheFixture(t)
	ctx := context.Background()
	s.Set(Prefix+"first", "old", 10)
	s.Set(Prefix+"second", "old", 10)
	require.NoError(t, c.DelCacheCtx(ctx, "first", "second"))
	for _, key := range []string{"first", "second"} {
		value, _ := s.Value(Prefix + key)
		require.Empty(t, value)
	}
	s.Fail("DEL", false)
	before := s.Calls("DEL")
	require.Error(t, c.DelCacheCtx(ctx, "first", "second"))
	require.Equal(t, before+2, s.Calls("DEL"), "attempt all invalidations on partial failure")
	_, err := c.ExecCtx(ctx, func(context.Context, sqlstore.SqlConn) (sql.Result, error) { return nil, nil }, "first")
	require.NoError(t, err, "cache failure cannot turn a committed write into a retryable failure")
}
