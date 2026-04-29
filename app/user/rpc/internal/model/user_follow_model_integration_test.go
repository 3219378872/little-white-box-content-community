//go:build integration

package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedProfilesAndFollows(t *testing.T) {
	t.Helper()
	conn := newTestConn()
	_, err := conn.ExecCtx(context.Background(),
		"INSERT INTO user_profile (id, username, password) VALUES (?, ?, ?)",
		1, "alice", "pw")
	require.NoError(t, err)
	_, err = conn.ExecCtx(context.Background(),
		"INSERT INTO user_profile (id, username, password) VALUES (?, ?, ?)",
		2, "bob", "pw")
	require.NoError(t, err)
	_, err = conn.ExecCtx(context.Background(),
		"INSERT INTO user_profile (id, username, password) VALUES (?, ?, ?)",
		3, "carol", "pw")
	require.NoError(t, err)
	// alice follows bob and carol
	_, err = conn.ExecCtx(context.Background(),
		"INSERT INTO user_follow (id, user_id, target_user_id) VALUES (?, ?, ?)",
		1, 1, 2)
	require.NoError(t, err)
	_, err = conn.ExecCtx(context.Background(),
		"INSERT INTO user_follow (id, user_id, target_user_id) VALUES (?, ?, ?)",
		2, 1, 3)
	require.NoError(t, err)
	// bob follows alice
	_, err = conn.ExecCtx(context.Background(),
		"INSERT INTO user_follow (id, user_id, target_user_id) VALUES (?, ?, ?)",
		3, 2, 1)
	require.NoError(t, err)
}

func TestUserFollowModelFindFollowers(t *testing.T) {
	testEnv.TruncateAll(t, "user_follow", "user_profile")
	seedProfilesAndFollows(t)

	model := NewUserFollowModel(newTestConn())

	// alice(UserId=1) is followed by bob, so alice has 1 follower
	followers, err := model.FindFollowers(context.Background(), 1, 0, 10)
	require.NoError(t, err)
	require.Len(t, followers, 1)
	assert.Equal(t, int64(2), followers[0].Id)
	assert.Equal(t, "bob", followers[0].Username)
}

func TestUserFollowModelFindFollowing(t *testing.T) {
	testEnv.TruncateAll(t, "user_follow", "user_profile")
	seedProfilesAndFollows(t)

	model := NewUserFollowModel(newTestConn())

	// alice follows bob and carol
	following, err := model.FindFollowing(context.Background(), 1, 0, 10)
	require.NoError(t, err)
	require.Len(t, following, 2)
}

func TestUserFollowModelCountFollowers(t *testing.T) {
	testEnv.TruncateAll(t, "user_follow", "user_profile")
	seedProfilesAndFollows(t)

	model := NewUserFollowModel(newTestConn())

	count, err := model.CountFollowers(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestUserFollowModelCountFollowing(t *testing.T) {
	testEnv.TruncateAll(t, "user_follow", "user_profile")
	seedProfilesAndFollows(t)

	model := NewUserFollowModel(newTestConn())

	count, err := model.CountFollowing(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}
