//go:build integration

package cachedstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"esx/pkg/modelcache"
	"esx/pkg/redisstore"
	"esx/pkg/sqlstore"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
)

func TestIndexInvalidationTransactionsAndCacheFailure(t *testing.T) {
	env := testutil.SetupTestEnv(t, "cache_migration", testutil.SchemaPath("xbh_content.sql"))
	defer env.Close()
	ctx := context.Background()
	conn := sqlstore.NewSqlConnFromDB(env.DB)
	_, err := conn.ExecCtx(ctx, "CREATE TABLE cache_probe (id BIGINT PRIMARY KEY, name VARCHAR(64) UNIQUE, value VARCHAR(64))")
	require.NoError(t, err)
	_, err = conn.ExecCtx(ctx, "INSERT INTO cache_probe VALUES (9007199254740993, 'lookup', 'before')")
	require.NoError(t, err)
	c := NewConn(conn, modelcache.CacheConf{{RedisConf: redisstore.RedisConf{Host: env.RedisAddr}}})
	defer c.redis.Close()
	type row struct {
		ID    int64  `db:"id"`
		Name  string `db:"name"`
		Value string `db:"value"`
	}
	primaryKey := func(id any) string { return fmt.Sprint("probe:id:", id) }
	load := func(name string) (row, error) {
		var value row
		err := c.QueryRowIndexCtx(ctx, &value, "probe:name:"+name, primaryKey, func(ctx context.Context, conn sqlstore.SqlConn, out any) (any, error) {
			err := conn.QueryRowCtx(ctx, out, "SELECT id,name,value FROM cache_probe WHERE name=?", name)
			return out.(*row).ID, err
		}, func(ctx context.Context, conn sqlstore.SqlConn, out, id any) error {
			return conn.QueryRowCtx(ctx, out, "SELECT id,name,value FROM cache_probe WHERE id=?", id)
		})
		return value, err
	}
	for range 2 {
		value, err := load("lookup")
		require.NoError(t, err)
		require.Equal(t, "before", value.Value)
		require.Equal(t, int64(9007199254740993), value.ID)
	}
	// A direct primary-key update must also invalidate reads through an index.
	_, err = c.ExecCtx(ctx, func(ctx context.Context, conn sqlstore.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, "UPDATE cache_probe SET value='after' WHERE id=9007199254740993")
	}, primaryKey(int64(9007199254740993)))
	require.NoError(t, err)
	value, err := load("lookup")
	require.NoError(t, err)
	require.Equal(t, "after", value.Value)
	rollback := errors.New("rollback requested")
	err = c.TransactCtx(ctx, func(ctx context.Context, s sqlstore.Session) error {
		_, err := s.ExecCtx(ctx, "UPDATE cache_probe SET value='rolled back'")
		require.NoError(t, err)
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	var raw string
	require.NoError(t, conn.QueryRowCtx(ctx, &raw, "SELECT value FROM cache_probe"))
	require.Equal(t, "after", raw)
	_, err = load("missing")
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = load("missing")
	require.ErrorIs(t, err, sql.ErrNoRows)
	ttl, err := env.Redis.TtlCtx(ctx, Prefix+"probe:name:missing")
	require.NoError(t, err)
	require.Greater(t, ttl, 0)
	require.LessOrEqual(t, ttl, 60)
	// A failed cache cannot make an already committed write look unsuccessful.
	require.NoError(t, c.redis.Close())
	_, err = c.ExecCtx(ctx, func(ctx context.Context, conn sqlstore.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, "UPDATE cache_probe SET value='cache offline'")
	}, primaryKey(int64(9007199254740993)))
	require.NoError(t, err)
	value, err = load("lookup")
	require.NoError(t, err)
	require.Equal(t, "cache offline", value.Value)
}
