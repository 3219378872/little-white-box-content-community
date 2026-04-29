//go:build integration

package model

import (
	"os"
	"testing"

	"esx/pkg/testutil"
	"util"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_user", testutil.SchemaPath("xbh_user.sql"))
	_ = util.InitSnowflake(1, 1)
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func newTestConn() sqlx.SqlConn {
	return sqlx.NewSqlConnFromDB(testEnv.DB)
}
