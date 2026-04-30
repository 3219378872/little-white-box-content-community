package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	chmodule "github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

type ClickHouseEnv struct {
	DB      *sql.DB
	DSN     string
	closeFn func()
}

func SetupClickHouseEnv(t *testing.T, initScripts ...string) *ClickHouseEnv {
	t.Helper()
	env, err := setupClickHouseEnv(initScripts...)
	require.NoError(t, err)
	return env
}

func SetupClickHouseEnvM(initScripts ...string) *ClickHouseEnv {
	env, err := setupClickHouseEnv(initScripts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupClickHouseEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

func setupClickHouseEnv(initScripts ...string) (*ClickHouseEnv, error) {
	ctx := context.Background()

	opts := []testcontainers.ContainerCustomizer{
		chmodule.WithDatabase("xbh_analytics"),
		chmodule.WithUsername("default"),
		chmodule.WithPassword(""),
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

func (e *ClickHouseEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}

func ClickHouseSchemaPath(filename string) string {
	_, f, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(f), "..", "..")
	return filepath.Join(root, "deploy", "sql", filename)
}
