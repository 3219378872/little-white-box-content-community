package middleware

import (
	"context"
	"testing"

	"jwtx"

	"github.com/stretchr/testify/assert"
)

func TestContextKeyCompat(t *testing.T) {
	// middleware 写入
	ctx := context.Background()
	ctx = jwtx.WithUserIdContext(ctx, 42)
	ctx = jwtx.WithUsernameContext(ctx, "testuser")

	// middleware 读取
	uid := GetUserId(ctx)
	assert.Equal(t, int64(42), uid)

	username := GetUsername(ctx)
	assert.Equal(t, "testuser", username)

	// jwtx 读取
	uid2, ok := jwtx.GetOptionalUserIdFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, int64(42), uid2)

	username2, ok := jwtx.GetUsernameFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "testuser", username2)
}
