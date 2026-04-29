package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	mysqlcontainer "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type TestEnv struct {
	DB       *sql.DB
	Redis    *redis.Redis
	MySQLDSN string
	closeFn  func()
}

// SetupTestEnv 启动 MySQL 8.0 + Redis 7 容器，返回统一测试环境。
func SetupTestEnv(t *testing.T, schemaPath string) *TestEnv {
	t.Helper()
	ctx := context.Background()

	// MySQL
	mysqlContainer, err := mysqlcontainer.Run(ctx,
		"mysql:8.0",
		mysqlcontainer.WithDatabase("testdb"),
		mysqlcontainer.WithUsername("root"),
		mysqlcontainer.WithPassword("testpass"),
		mysqlcontainer.WithScripts(schemaPath),
		testcontainers.WithEnv(map[string]string{
			"TZ":   "Asia/Shanghai",
			"LANG": "C.UTF-8",
		}),
		testcontainers.WithCmd(
			"--default-authentication-plugin=mysql_native_password",
			"--character-set-server=utf8mb4",
			"--collation-server=utf8mb4_unicode_ci",
			"--sql-mode=STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION",
		),
	)
	require.NoError(t, err)

	dsn, err := mysqlContainer.ConnectionString(ctx,
		"charset=utf8mb4", "parseTime=true", "loc=Asia%2FShanghai")
	require.NoError(t, err)

	db, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(ctx))

	// Redis
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}
	redisContainer, err := testcontainers.GenericContainer(ctx, req)
	require.NoError(t, err)

	redisHost, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	require.NoError(t, err)

	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort.Port())
	rds := redis.MustNewRedis(redis.RedisConf{
		Host: redisAddr,
		Type: redis.NodeType,
	})

	cleanup := func() {
		_ = db.Close()
		_ = testcontainers.TerminateContainer(mysqlContainer)
		_ = testcontainers.TerminateContainer(redisContainer)
	}

	return &TestEnv{
		DB:       db,
		Redis:    rds,
		MySQLDSN: dsn,
		closeFn:  cleanup,
	}
}

func (e *TestEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}

// TruncateAll 清空指定表。
func (e *TestEnv) TruncateAll(t *testing.T, tables ...string) {
	t.Helper()
	_, err := e.DB.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 0")
	require.NoError(t, err)
	for _, table := range tables {
		_, err := e.DB.ExecContext(context.Background(), "TRUNCATE TABLE "+table)
		require.NoError(t, err)
	}
	_, err = e.DB.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1")
	require.NoError(t, err)
}

// SchemaPath 返回项目根目录下的 deploy/sql/ 路径。
func SchemaPath(filename string) string {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..")
	return filepath.Join(root, "deploy", "sql", filename)
}
