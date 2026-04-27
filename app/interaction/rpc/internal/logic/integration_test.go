//go:build integration

package logic

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	"esx/app/interaction/internal/config"
	"esx/app/interaction/internal/svc"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

var (
	testSvcCtx *svc.ServiceContext
	testDB     *sql.DB
)

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func TestMain(m *testing.M) {
	dsn := getEnv("TEST_MYSQL_DSN", getEnv("DB_INTERACTION", ""))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "错误: TEST_MYSQL_DSN 或 DB_INTERACTION 环境变量必须设置")
		os.Exit(1)
	}
	redisHost := getEnv("TEST_REDIS_HOST", "127.0.0.1:6379")
	redisPass := getEnv("TEST_REDIS_PASS", getEnv("REDIS_PASS", ""))

	cfg := config.Config{
		RpcServerConf: zrpc.RpcServerConf{},
		DataSource:    dsn,
	}
	cfg.Redis.RedisConf = redis.RedisConf{
		Host: redisHost,
		Pass: redisPass,
		Type: "node",
	}

	testSvcCtx = svc.NewServiceContext(cfg)

	var err error
	testDB, err = sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "打开数据库失败: %v\n", err)
		os.Exit(1)
	}

	if err := testDB.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "数据库连接失败: %v\n", err)
		os.Exit(1)
	}

	ensureActionCountTable()

	if _, err := testSvcCtx.Redis.Exists("integration:ping"); err != nil {
		fmt.Fprintf(os.Stderr, "Redis 连接失败: %v\n", err)
		os.Exit(1)
	}

	resetIntegrationState()
	code := m.Run()
	resetIntegrationState()
	_ = testDB.Close()
	os.Exit(code)
}

func resetIntegrationState() {
	for _, table := range []string{"like_record", "favorite", "action_count", "favorite_folder", "view_history", "report"} {
		if _, err := testDB.Exec(fmt.Sprintf("DELETE FROM `%s`", table)); err != nil {
			fmt.Fprintf(os.Stderr, "清理 %s 失败: %v\n", table, err)
			os.Exit(1)
		}
	}

	for _, key := range []string{
		"interaction:action_count:900001:1",
		"interaction:action_count:910101:1",
		"interaction:action_count:920001:1",
	} {
		if _, err := testSvcCtx.Redis.Del(key); err != nil {
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
	if _, err := testDB.Exec(ddl); err != nil {
		fmt.Fprintf(os.Stderr, "创建 action_count 失败: %v\n", err)
		os.Exit(1)
	}
}
