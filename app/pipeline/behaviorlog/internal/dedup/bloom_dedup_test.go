package dedup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func setupRedis(t *testing.T) *redis.Redis {
	t.Helper()
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
	require.NoError(t, err)
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(container) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err)

	return redis.MustNewRedis(redis.RedisConf{
		Host: fmt.Sprintf("%s:%s", host, port.Port()),
		Type: redis.NodeType,
	})
}

func TestBloomDedup_NewEvent_NotDuplicate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	dup, err := d.IsDuplicate(context.Background(), "event-001")
	require.NoError(t, err)
	assert.False(t, dup)
}

func TestBloomDedup_SameEvent_IsDuplicate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	dup1, err := d.IsDuplicate(context.Background(), "event-002")
	require.NoError(t, err)
	assert.False(t, dup1)

	dup2, err := d.IsDuplicate(context.Background(), "event-002")
	require.NoError(t, err)
	assert.True(t, dup2)
}

func TestBloomDedup_DifferentEvents_NotDuplicate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	_, _ = d.IsDuplicate(context.Background(), "event-aaa")
	dup, err := d.IsDuplicate(context.Background(), "event-bbb")
	require.NoError(t, err)
	assert.False(t, dup)
}

func TestBloomDedup_KeyContainsDate(t *testing.T) {
	rds := setupRedis(t)
	d := NewBloomDedup(rds, 1024)

	key := d.keyForDate(time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "bf:behavior_events:20260429", key)
}

func TestBloomDedup_RedisError_ReturnsError(t *testing.T) {
	rds := redis.MustNewRedis(redis.RedisConf{
		Host:     "127.0.0.1:1",
		Type:     redis.NodeType,
		NonBlock: true,
	})
	d := NewBloomDedup(rds, 1024)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := d.IsDuplicate(ctx, "event-unavailable")
	assert.Error(t, err)
}
