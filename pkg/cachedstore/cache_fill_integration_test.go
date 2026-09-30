//go:build integration

package cachedstore

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"esx/pkg/modelcache"
	"esx/pkg/redisstore"
	"esx/pkg/sqlstore"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
)

// This complements the deterministic RESP tests by executing the actual Lua
// against Redis and taking the snapshots and committed changes through MySQL.
func TestCacheFillRaceWithRedisAndSQL(t *testing.T) {
	env := testutil.SetupTestEnv(t, "cache_fill_fence", testutil.SchemaPath("xbh_content.sql"))
	defer env.Close()
	ctx := context.Background()
	conn := sqlstore.NewSqlConnFromDB(env.DB)
	_, err := conn.ExecCtx(ctx, "CREATE TABLE cache_fill_probe (id BIGINT PRIMARY KEY, status BIGINT NOT NULL)")
	require.NoError(t, err)
	c := NewConn(conn, modelcache.CacheConf{{RedisConf: redisstore.RedisConf{Host: env.RedisAddr}}})
	defer c.redis.Close()
	for id, tc := range []struct {
		name     string
		before   bool
		mutation string
		status   int
		missing  bool
	}{
		{name: "update", before: true, mutation: "UPDATE cache_fill_probe SET status=2 WHERE id=?", status: 2},
		{name: "soft delete", before: true, mutation: "UPDATE cache_fill_probe SET status=0 WHERE id=?"},
		{name: "hard delete", before: true, mutation: "DELETE FROM cache_fill_probe WHERE id=?", missing: true},
		{name: "create", mutation: "INSERT INTO cache_fill_probe (id,status) VALUES (?,1)", status: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.before {
				_, err := conn.ExecCtx(ctx, "INSERT INTO cache_fill_probe VALUES (?,1)", id)
				require.NoError(t, err)
			}
			key := fmt.Sprintf("live:row:%d", id)
			loaded, release := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			load := func(ctx context.Context, conn sqlstore.SqlConn, out any) error {
				return conn.QueryRowCtx(ctx, out, "SELECT status FROM cache_fill_probe WHERE id=?", id)
			}
			go func() {
				var row cacheRow
				done <- c.QueryRowCtx(ctx, &row, key, func(ctx context.Context, conn sqlstore.SqlConn, out any) error {
					err := load(ctx, conn, out)
					close(loaded)
					<-release
					return err
				})
			}()
			<-loaded
			_, mutationErr := c.ExecCtx(ctx, func(ctx context.Context, conn sqlstore.SqlConn) (sql.Result, error) {
				return conn.ExecCtx(ctx, tc.mutation, id)
			}, key)
			close(release)
			require.NoError(t, mutationErr)
			oldErr := <-done
			if tc.before {
				require.NoError(t, oldErr)
			} else {
				require.ErrorIs(t, oldErr, ErrNotFound)
			}
			var fresh cacheRow
			err := c.QueryRowCtx(ctx, &fresh, key, load)
			if tc.missing {
				require.ErrorIs(t, err, ErrNotFound)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.status, fresh.Status)
			}
			ttl, err := c.redis.TtlCtx(ctx, Prefix+key)
			require.NoError(t, err)
			require.Positive(t, ttl)
			limit := c.options.TTLSeconds
			if tc.missing {
				limit = c.options.NotFoundTTLSeconds
			}
			require.LessOrEqual(t, ttl, limit)
		})
	}
	// An expired marker cannot overwrite a replacement marker, even though the
	// same key has become available again. This executes the exact production Lua.
	key := Prefix + "live:expired"
	old := c.reserveFill(ctx, key)
	require.NotEmpty(t, old)
	require.NoError(t, c.redis.ExpireCtx(ctx, key, -1))
	current := c.reserveFill(ctx, key)
	require.NotEmpty(t, current)
	require.NotEqual(t, old, current)
	c.finishFill(ctx, key, old, &cacheRow{Status: 1}, nil)
	value, err := c.redis.GetCtx(ctx, key)
	require.NoError(t, err)
	require.Equal(t, current, value)
	c.finishFill(ctx, key, current, &cacheRow{Status: 2}, nil)
	value, err = c.redis.GetCtx(ctx, key)
	require.NoError(t, err)
	require.Contains(t, value, `"Status":2`)
}
