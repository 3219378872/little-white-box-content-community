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

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	chmodule "github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ClickHouseEnv 是一次性 ClickHouse 容器及其连接。
type ClickHouseEnv struct {
	DB      *sql.DB
	DSN     string
	closeFn func()
}

// SetupClickHouseEnv 为单个测试启动 ClickHouse 并执行初始化脚本，失败时让测试失败。
func SetupClickHouseEnv(t *testing.T, initScripts ...string) *ClickHouseEnv {
	t.Helper()
	env, err := setupClickHouseEnv(initScripts...)
	require.NoError(t, err)
	return env
}

// SetupClickHouseEnvM 供 TestMain 使用，启动失败直接退出进程。
func SetupClickHouseEnvM(initScripts ...string) *ClickHouseEnv {
	env, err := setupClickHouseEnv(initScripts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupClickHouseEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

// setupClickHouseEnv 启动容器、等待就绪、执行初始化脚本并返回连接。
func setupClickHouseEnv(initScripts ...string) (*ClickHouseEnv, error) {
	ctx := context.Background()

	opts := []testcontainers.ContainerCustomizer{
		chmodule.WithDatabase("xbh_analytics"),
		chmodule.WithUsername("default"),
		chmodule.WithPassword(""),
		testcontainers.WithWaitStrategyAndDeadline(
			5*time.Minute,
			wait.ForHTTP("/").
				WithPort("8123/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status == 200 }).
				WithStartupTimeout(5*time.Minute),
		),
	}
	for _, script := range initScripts {
		opts = append(opts, chmodule.WithInitScripts(script))
	}

	container, err := chmodule.Run(ctx, "clickhouse/clickhouse-server:23.8-alpine", opts...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("clickhouse dsn: %w", err)
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("sql.Open clickhouse: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = testcontainers.TerminateContainer(container)
	}

	return &ClickHouseEnv{DB: db, DSN: dsn, closeFn: cleanup}, nil
}

// Close 关闭连接并终止容器。
func (e *ClickHouseEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}

// ClickHouseSchemaPath 返回仓库 deploy/sql 下的建表脚本路径，与调用目录无关。
func ClickHouseSchemaPath(filename string) string {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..")
	return filepath.Join(root, "deploy", "sql", filename)
}
