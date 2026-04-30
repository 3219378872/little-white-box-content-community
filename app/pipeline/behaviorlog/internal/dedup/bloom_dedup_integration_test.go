//go:build integration

package dedup_test

import (
	"context"
	"os"
	"testing"

	"esx/app/pipeline/behaviorlog/internal/dedup"
	"esx/app/pipeline/behaviorlog/internal/svc"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_test_behaviorlog_dedup", testutil.SchemaPath("xbh_user.sql"))

	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func integrationRedis() *redis.Redis {
	return testEnv.Redis
}

func TestBloomDedup_NewEvent_NotDuplicate(t *testing.T) {
	rds := integrationRedis()
	d := dedup.NewBloomDedup(svc.NewRedisBloomStore(rds), 1024)

	dup, err := d.IsDuplicate(context.Background(), "event-001")
	require.NoError(t, err)
	assert.False(t, dup)
}

func TestBloomDedup_SameEvent_IsDuplicate(t *testing.T) {
	rds := integrationRedis()
	d := dedup.NewBloomDedup(svc.NewRedisBloomStore(rds), 1024)

	dup1, err := d.IsDuplicate(context.Background(), "event-002")
	require.NoError(t, err)
	assert.False(t, dup1)

	require.NoError(t, d.MarkProcessed(context.Background(), "event-002"))

	dup2, err := d.IsDuplicate(context.Background(), "event-002")
	require.NoError(t, err)
	assert.True(t, dup2)
}

func TestBloomDedup_DifferentEvents_NotDuplicate(t *testing.T) {
	rds := integrationRedis()
	d := dedup.NewBloomDedup(svc.NewRedisBloomStore(rds), 1024)

	require.NoError(t, d.MarkProcessed(context.Background(), "event-aaa"))
	dup, err := d.IsDuplicate(context.Background(), "event-bbb")
	require.NoError(t, err)
	assert.False(t, dup)
}
