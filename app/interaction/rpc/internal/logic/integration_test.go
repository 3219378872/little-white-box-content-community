//go:build integration

package logic

import (
	"context"
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

	ensureActionCountTable()

	resetIntegrationState()
	code := m.Run()
	resetIntegrationState()
	testEnv.Close()
	os.Exit(code)
}

func resetIntegrationState() {
	for _, table := range []string{"like_record", "favorite", "action_count", "favorite_folder", "view_history", "report"} {
		if _, err := testEnv.DB.Exec(fmt.Sprintf("DELETE FROM `%s`", table)); err != nil {
			fmt.Fprintf(os.Stderr, "清理 %s 失败: %v\n", table, err)
			os.Exit(1)
		}
	}

	for _, key := range []string{
		"interaction:action_count:900001:1",
		"interaction:action_count:910101:1",
		"interaction:action_count:920001:1",
	} {
		if _, err := testSvcCtx.Redis.DelCtx(context.Background(), key); err != nil {
			fmt.Fprintf(os.Stderr, "清理 Redis key %s 失败: %v\n", key, err)
			os.Exit(1)
		}
	}
}

func ensureActionCountTable() {
	const ddl = `
CREATE TABLE IF NOT EXISTS action_count (
    id BIGINT NOT NULL AUTO_INCREMENT,
    target_id BIGINT NOT NULL,
    target_type TINYINT NOT NULL,
    like_count BIGINT NOT NULL DEFAULT 0,
    favorite_count BIGINT NOT NULL DEFAULT 0,
    comment_count BIGINT NOT NULL DEFAULT 0,
    share_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uk_target (target_id, target_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`
	if _, err := testEnv.DB.Exec(ddl); err != nil {
		fmt.Fprintf(os.Stderr, "创建 action_count 失败: %v\n", err)
		os.Exit(1)
	}
}
