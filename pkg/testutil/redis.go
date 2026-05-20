package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// RedisEnv 是只起 Redis 的轻量集成测试环境，
// 适用于不需要 MySQL 的纯缓存/计数场景（content-cleanup、content-stat 等）。
type RedisEnv struct {
	Redis   *redis.Redis
	Addr    string
	closeFn func()
}

func SetupRedisEnv(t *testing.T) *RedisEnv {
	t.Helper()
	env, err := setupRedisEnv()
	require.NoError(t, err)
	return env
}

func SetupRedisEnvM() *RedisEnv {
	env, err := setupRedisEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SetupRedisEnvM: %v\n", err)
		os.Exit(1)
	}
	return env
}

func setupRedisEnv() (*RedisEnv, error) {
	ctx := context.Background()
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections").WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}
	container, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("redis container: %w", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("redis host: %w", err)
	}
	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		return nil, fmt.Errorf("redis port: %w", err)
	}
	addr := fmt.Sprintf("%s:%s", host, port.Port())
	rds := redis.MustNewRedis(redis.RedisConf{Host: addr, Type: redis.NodeType})
	cleanup := func() { _ = testcontainers.TerminateContainer(container) }
	return &RedisEnv{Redis: rds, Addr: addr, closeFn: cleanup}, nil
}

func (e *RedisEnv) Close() {
	if e.closeFn != nil {
		e.closeFn()
	}
}
