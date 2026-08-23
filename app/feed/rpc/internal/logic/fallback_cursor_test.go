package logic

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackCursorRoundTripAndTamperDetection(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	token, err := encodeFallbackCursor("test-secret", "request-1", 3, "hot-c", "latest-c", now)
	require.NoError(t, err)

	page, hotCursor, latestCursor, matched, err := decodeFallbackCursor("test-secret", token, "request-1", now.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, matched)
	assert.Equal(t, int32(3), page)
	assert.Equal(t, "hot-c", hotCursor)
	assert.Equal(t, "latest-c", latestCursor)

	_, _, _, matched, err = decodeFallbackCursor("wrong-secret", token, "request-1", now)
	assert.True(t, matched)
	assert.Error(t, err)
}

func TestFallbackCursorRejectsCrossRequestAndExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	token, err := encodeFallbackCursor("test-secret", "request-1", 2, "", "", now)
	require.NoError(t, err)

	_, _, _, _, err = decodeFallbackCursor("test-secret", token, "request-2", now)
	assert.Error(t, err)
	_, _, _, _, err = decodeFallbackCursor("test-secret", token, "request-1", now.Add(time.Hour))
	assert.Error(t, err)
}

func TestFallbackCursorIgnoresRecommendCursor(t *testing.T) {
	_, _, _, matched, err := decodeFallbackCursor("test-secret", "recommend-token", "request-1", time.Now())
	assert.NoError(t, err)
	assert.False(t, matched)
}
