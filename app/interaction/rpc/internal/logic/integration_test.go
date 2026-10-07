//go:build integration

package logic

import (
	"esx/app/interaction/rpc/internal/config"
	"esx/app/interaction/rpc/internal/svc"
	"esx/pkg/testutil"
	"fmt"
	"os"
	"testing"

	redis "esx/pkg/redisstore"
	"esx/pkg/rpcx"

	_ "github.com/go-sql-driver/mysql"
)

var (
	testEnv    *testutil.TestEnv
	testSvcCtx *svc.ServiceContext
)

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_interaction", testutil.SchemaPath("xbh_interaction.sql"))

	cfg := config.Config{
		InternalSecret: "test-internal-secret",
		RpcServerConf:  rpcx.RpcServerConf{},
		DataSource:     testEnv.MySQLDSN,
	}
	cfg.Redis.RedisConf = redis.RedisConf{
		Host: testEnv.RedisAddr,
		Type: "node",
	}

	testSvcCtx = svc.NewServiceContext(cfg)
	testSvcCtx.ContentService = &fakeContentService{}

	resetIntegrationState()
	code := m.Run()
	resetIntegrationState()
	testEnv.Close()
	os.Exit(code)
}

func resetIntegrationState() {
	for _, table := range []string{"like_record", "favorite", "action_count", "favorite_folder", "view_history", "report", "event_outbox"} {
		if _, err := testEnv.DB.Exec(fmt.Sprintf("DELETE FROM `%s`", table)); err != nil {
			fmt.Fprintf(os.Stderr, "清理 %s 失败: %v\n", table, err)
			os.Exit(1)
		}
	}
}
