package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDedupRetentionIndependentOfFeatureRetention(t *testing.T) {
	for _, featureTTL := range []int{3600, 2592000} {
		r := &fakeEvaler{}
		s := NewRedisBehaviorStoreWithRetention(r, "v2", "recommend", featureTTL, 7776000, enabledPreferences())
		behavior := featureBehavior()
		behavior.Action = "exposure"
		behavior.RequestID = "request"
		behavior.Position = new(int32(1))
		require.NoError(t, s.Record(context.Background(), behavior))
		require.Equal(t, featureTTL, r.args[0])
		require.Equal(t, 7776000, r.args[11])
		require.Contains(t, recordFeatureScript, "redis.call('SETEX', KEYS[1], ARGV[12], 'v3')")
		require.Contains(t, recordFeatureScript, "redis.call('SETEX', KEYS[2], ARGV[12], 'v3')")
		require.Contains(t, recordFeatureScript, "redis.call('EXPIRE', KEYS[3], ARGV[1])")
	}
}
func TestUnsafeDedupRetentionRejectedBeforeWriting(t *testing.T) {
	r := &fakeEvaler{}
	s := NewRedisBehaviorStoreWithRetention(r, "v2", "recommend", 2592000, 2592000, enabledPreferences())
	require.Error(t, s.Record(context.Background(), featureBehavior()))
	require.Empty(t, r.keys)
}
