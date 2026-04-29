//go:build integration

package logic

import (
	"os"
	"testing"

	"esx/pkg/testutil"
	"user/internal/model"
	"user/internal/svc"
	"util"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var testEnv *testutil.TestEnv
var testSvcCtx *svc.ServiceContext

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_user", testutil.SchemaPath("xbh_user.sql"))
	testSvcCtx = buildSvcCtx(testEnv)
	_ = util.InitSnowflake(1, 1)
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func buildSvcCtx(env *testutil.TestEnv) *svc.ServiceContext {
	conn := sqlx.NewSqlConnFromDB(env.DB)
	return &svc.ServiceContext{
		DB:                conn,
		UserProfileModel:  model.NewUserProfileModel(conn),
		UserFollowModel:   model.NewUserFollowModel(conn),
		UserLoginLogModel: model.NewUserLoginLogModel(conn),
		RedisClient:       env.Redis,
	}
}
