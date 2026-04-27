//go:build integration

package model

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func newFeedTestDB(t *testing.T) (sqlx.SqlConn, func()) {
	t.Helper()
	ctx := context.Background()
	root, err := filepath.Abs("../../../..")
	require.NoError(t, err)
	scriptPath := filepath.Join(root, "deploy", "sql", "xbh_feed.sql")
	password := os.Getenv("MYSQL_ROOT_PASSWORD")
	if password == "" {
		password = "Xbh@MySQL2024!"
	}
	container, err := mysqlcontainer.Run(ctx,
		"mysql:8.0",
		mysqlcontainer.WithDatabase("xbh_feed"),
		mysqlcontainer.WithUsername("root"),
		mysqlcontainer.WithPassword(password),
		mysqlcontainer.WithScripts(scriptPath),
		testcontainers.WithEnv(map[string]string{
			"TZ":   "Asia/Shanghai",
			"LANG": "C.UTF-8",
		}),
		testcontainers.WithCmd("--default-authentication-plugin=mysql_native_password", "--character-set-server=utf8mb4", "--collation-server=utf8mb4_unicode_ci"),
	)
	require.NoError(t, err)

	dsn, err := container.ConnectionString(ctx, "charset=utf8mb4", "parseTime=true", "loc=Asia%2FShanghai")
	require.NoError(t, err)
	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(ctx))

	cleanup := func() {
		_ = db.Close()
		require.NoError(t, testcontainers.TerminateContainer(container))
	}
	return sqlx.NewSqlConnFromDB(db), cleanup
}
