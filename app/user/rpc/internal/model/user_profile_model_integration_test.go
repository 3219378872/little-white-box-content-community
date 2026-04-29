//go:build integration

package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserProfileModelUpdateUserDes(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	conn := newTestConn()
	_, err := conn.ExecCtx(context.Background(),
		"INSERT INTO user_profile (id, username, password) VALUES (?, ?, ?)",
		1, "testuser", "pw")
	require.NoError(t, err)

	model := NewUserProfileModel(conn)
	err = model.UpdateUserDes(context.Background(), 1, "nick", "http://av.jpg", "bio text")
	require.NoError(t, err)

	// 验证更新结果
	p, err := model.FindOne(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, "nick", p.Nickname.String)
	assert.Equal(t, "http://av.jpg", p.AvatarUrl.String)
	assert.Equal(t, "bio text", p.Bio.String)
}
