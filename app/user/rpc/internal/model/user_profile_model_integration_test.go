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

func TestUserProfileModelFindByIDs(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	ctx := context.Background()
	conn := newTestConn()
	for _, user := range []struct {
		id       int64
		username string
	}{
		{id: 1, username: "alice"},
		{id: 2, username: "bob"},
		{id: 3, username: "carol"},
	} {
		_, err := conn.ExecCtx(ctx,
			"INSERT INTO user_profile (id, username, password) VALUES (?, ?, ?)",
			user.id, user.username, "pw")
		require.NoError(t, err)
	}

	profiles, err := NewUserProfileModel(conn).FindByIDs(ctx, []int64{3, 1, 404})
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	usernames := make(map[int64]string, len(profiles))
	for _, profile := range profiles {
		usernames[profile.Id] = profile.Username
	}
	assert.Equal(t, map[int64]string{1: "alice", 3: "carol"}, usernames)

	profiles, err = NewUserProfileModel(conn).FindByIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestUserProfileModelSearchPublicMatchesProfileFields(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")
	ctx := context.Background()
	conn := newTestConn()
	for _, user := range []struct {
		id        int64
		username  string
		nickname  string
		bio       string
		followers int64
		status    int
	}{
		{id: 1, username: "golang-dev", followers: 5, status: 1},
		{id: 2, username: "alice", nickname: "Go Teacher", followers: 20, status: 1},
		{id: 3, username: "bob", bio: "writes go services", followers: 10, status: 1},
		{id: 4, username: "go-disabled", followers: 100, status: 0},
	} {
		_, err := conn.ExecCtx(ctx, `
			INSERT INTO user_profile (id, username, password, nickname, bio, follower_count, status)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			user.id, user.username, "pw", user.nickname, user.bio, user.followers, user.status)
		require.NoError(t, err)
	}

	profiles, total, err := NewUserProfileModel(conn).SearchPublic(ctx, "go", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, profiles, 1)
	assert.Equal(t, int64(3), profiles[0].Id)
}
