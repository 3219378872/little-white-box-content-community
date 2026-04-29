package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
	DB        *sql.DB
	Redis     *redis.Redis
	RedisAddr string
	MySQLDSN  string
	closeFn   func()
}

// SetupTestEnv 启动 MySQL 8.0 + Redis 7 容器，返回统一测试环境。
func SetupTestEnv(t *testing.T, dbName, schemaPath string) *TestEnv {
	t.Helper()
	env, err := setupTestEnv(dbName, schemaPath)
	require.NoError(t, err)
	return env
}

// SetupTestEnvM for TestMain (*testing.M).
func SetupTestEnvM(dbName, schemaPath string) *TestEnv {
	env, err := setupTestEnv(dbName, schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupTestEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

func setupTestEnv(dbName, schemaPath string) (*TestEnv, error) {
	ctx := context.Background()

	// MySQL
	mysqlContainer, err := mysqlcontainer.Run(ctx,
		"mysql:8.0",
		mysqlcontainer.WithDatabase(dbName),
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
	if err != nil {
		return nil, fmt.Errorf("mysql container: %w", err)
	}

	dsn, err := mysqlContainer.ConnectionString(ctx,
		"charset=utf8mb4", "parseTime=true", "loc=Asia%2FShanghai")
	if err != nil {
		return nil, fmt.Errorf("mysql connection string: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}

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
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("redis container: %w", err)
	}

	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		db.Close()
		_ = testcontainers.TerminateContainer(redisContainer)
		return nil, fmt.Errorf("redis host: %w", err)
	}
	redisPort, err := redisContainer.MappedPort(ctx, "6379")
	if err != nil {
		db.Close()
		_ = testcontainers.TerminateContainer(redisContainer)
		return nil, fmt.Errorf("redis port: %w", err)
	}

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
		DB:        db,
		Redis:     rds,
		RedisAddr: redisAddr,
		MySQLDSN:  dsn,
		closeFn:   cleanup,
	}, nil
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
