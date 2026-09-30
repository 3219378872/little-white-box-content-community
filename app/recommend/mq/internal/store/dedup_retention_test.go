package store

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type retentionRedis struct {
	scans   []string
	keys    []string
	scanErr error
	evalErr error
	calls   []struct {
		keys []string
		args []any
	}
}

func (r *retentionRedis) ScanCtx(ctx context.Context, _ uint64, pattern string, _ int64) ([]string, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.scans = append(r.scans, pattern)
	return r.keys, 0, r.scanErr
}
func (r *retentionRedis) EvalCtx(_ context.Context, script string, keys []string, args ...any) (any, error) {
	if script != upgradeDedupRetentionScript {
		return nil, errors.New("unexpected script")
	}
	r.calls = append(r.calls, struct {
		keys []string
		args []any
	}{keys, args})
	return int64(1), r.evalErr
}
func TestDedupMigrationScansReceiptsAndPreservesAgeBudget(t *testing.T) {
	redis := &retentionRedis{keys: []string{"receipt-a"}}
	store := NewRedisBehaviorStore(redis, "v2", "recommend", 3600, enabledPreferences())
	n, err := store.UpgradeDedupRetention(context.Background(), 2592000)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, []string{"feature:v2:dedup:*", "feature:v2:u:*:exposure:dedup:*"}, redis.scans)
	for _, call := range redis.calls {
		require.Equal(t, []any{int64(2592000000), int64(7776000000)}, call.args)
	}
	require.Contains(t, upgradeDedupRetentionScript, "marker == 'v3'")
	require.Contains(t, upgradeDedupRetentionScript, "remaining + desired - legacy")
}
func TestDedupMigrationFailsClosedOnUnavailableOrInvalidState(t *testing.T) {
	for _, fault := range []string{"scan", "write", "cancel", "invalid_ttl"} {
		t.Run(fault, func(t *testing.T) {
			redis := &retentionRedis{keys: []string{"receipt-a"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ttl := 2592000
			switch fault {
			case "scan":
				redis.scanErr = errors.New("scan failed")
			case "write":
				redis.evalErr = errors.New("unknown receipt")
			case "cancel":
				cancel()
			case "invalid_ttl":
				ttl = 0
			}
			_, err := NewRedisBehaviorStore(redis, "v2", "recommend", 3600).UpgradeDedupRetention(ctx, ttl)
			require.Error(t, err)
		})
	}
	_, err := NewRedisBehaviorStore(&fakeEvaler{}, "v2", "recommend", 3600).UpgradeDedupRetention(context.Background(), 2592000)
	require.ErrorContains(t, err, "SCAN")
}
