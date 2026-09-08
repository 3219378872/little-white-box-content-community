package logic

import (
	"testing"
	"time"

	"esx/app/feed/rpc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackCursorRoundTripAndTamperDetection(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	binding := model.FallbackBinding{IdentityHash: "identity", RequestID: "request-1", Scene: "home", PageSize: 2}
	token, err := encodeFallbackCursor("test-secret", "state-1", binding, now.Add(fallbackCursorTTL).Unix(), now)
	require.NoError(t, err)

	stateID, expiresAt, matched, err := decodeFallbackCursor("test-secret", token, binding, now.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, matched)
	assert.Equal(t, "state-1", stateID)
	assert.Equal(t, now.Add(fallbackCursorTTL).Unix(), expiresAt)

	_, _, matched, err = decodeFallbackCursor("wrong-secret", token, binding, now)
	assert.True(t, matched)
	assert.Error(t, err)
}

func TestFallbackCursorRejectsCrossRequestAndExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	binding := model.FallbackBinding{IdentityHash: "identity", RequestID: "request-1", Scene: "home", SessionID: "session", ExperimentID: "exp", PageSize: 2}
	token, err := encodeFallbackCursor("test-secret", "state-1", binding, now.Add(fallbackCursorTTL).Unix(), now)
	require.NoError(t, err)

	for _, mutate := range []func(*model.FallbackBinding){
		func(b *model.FallbackBinding) { b.IdentityHash = "other" },
		func(b *model.FallbackBinding) { b.RequestID = "other" },
		func(b *model.FallbackBinding) { b.Scene = "other" },
		func(b *model.FallbackBinding) { b.SessionID = "other" },
		func(b *model.FallbackBinding) { b.ExperimentID = "other" },
		func(b *model.FallbackBinding) { b.PageSize = 3 },
	} {
		other := binding
		mutate(&other)
		_, _, _, err = decodeFallbackCursor("test-secret", token, other, now)
		assert.Error(t, err)
	}
	_, _, _, err = decodeFallbackCursor("test-secret", token, binding, now.Add(10*time.Minute))
	assert.Error(t, err)
}

func TestFallbackCursorIgnoresRecommendCursor(t *testing.T) {
	_, _, matched, err := decodeFallbackCursor("test-secret", "recommend-token", model.FallbackBinding{}, time.Now())
	assert.NoError(t, err)
	assert.False(t, matched)
}
