//go:build integration

package model

import (
	"context"
	"fmt"
	"testing"

	cache "esx/pkg/modelcache"
	"esx/pkg/redisstore"

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

func TestUserProfileModelCardCacheInvalidatedByProfileUpdate(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")
	ctx := context.Background()
	conn := newTestConn()
	_, err := conn.ExecCtx(ctx,
		"INSERT INTO user_profile (id, username, password, nickname, avatar_url, follower_count) VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, NULL, NULL, 0)",
		1, "alice", "pw", "Alice", "https://a/1.png", 3,
		2, "bob", "pw")
	require.NoError(t, err)

	model := NewCachedUserProfileModel(conn, cache.CacheConf{{RedisConf: redisstore.RedisConf{Host: testEnv.RedisAddr}}})
	cardKey := func(id int64) string { return fmt.Sprintf("cache:v3:user:card:%d", id) }
	for _, id := range []int64{1, 2, 404} {
		_, _ = testEnv.Redis.DelCtx(ctx, cardKey(id))
	}

	cards, err := model.FindCardsByIDs(ctx, []int64{2, 404, 1})
	require.NoError(t, err)
	require.Len(t, cards, 2)
	assert.Equal(t, UserCard{Id: 2, Username: "bob"}, *cards[0])
	assert.Equal(t, UserCard{Id: 1, Username: "alice", Nickname: "Alice", AvatarUrl: "https://a/1.png"}, *cards[1])

	cached, err := testEnv.Redis.GetCtx(ctx, cardKey(1))
	require.NoError(t, err)
	assert.Contains(t, cached, `"Nickname":"Alice"`)
	assert.NotContains(t, cached, "pw", "the card never carries the password hash")
	ttl, err := testEnv.Redis.TtlCtx(ctx, cardKey(1))
	require.NoError(t, err)
	assert.InDelta(t, userCardTTLSeconds, ttl, 5)
	missing, err := testEnv.Redis.GetCtx(ctx, cardKey(404))
	require.NoError(t, err)
	assert.Contains(t, missing, `"Missing":true`)
	missingTTL, err := testEnv.Redis.TtlCtx(ctx, cardKey(404))
	require.NoError(t, err)
	assert.InDelta(t, userCardNotFoundTTLSeconds, missingTTL, 5)

	// Counters are not card fields: a follow-style update leaves the card cached.
	_, err = conn.ExecCtx(ctx, "UPDATE user_profile SET follower_count = follower_count + 1 WHERE id = 1")
	require.NoError(t, err)
	cached, err = testEnv.Redis.GetCtx(ctx, cardKey(1))
	require.NoError(t, err)
	assert.NotEmpty(t, cached)

	require.NoError(t, model.UpdateUserDes(ctx, 1, "Alice 2", "https://a/2.png", "bio"))
	cached, err = testEnv.Redis.GetCtx(ctx, cardKey(1))
	require.NoError(t, err)
	assert.Empty(t, cached, "a profile write invalidates the card after commit")

	cards, err = model.FindCardsByIDs(ctx, []int64{1})
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Alice 2", cards[0].Nickname)
	assert.Equal(t, "https://a/2.png", cards[0].AvatarUrl)
}

func TestUserProfileModelSearchPublicPageSkipsCount(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")
	ctx := context.Background()
	conn := newTestConn()
	_, err := conn.ExecCtx(ctx,
		"INSERT INTO user_profile (id, username, password, status, follower_count) VALUES (1, 'gopher', 'pw', 1, 5), (2, 'go-fan', 'pw', 1, 9), (3, 'go-banned', 'pw', 0, 99)")
	require.NoError(t, err)

	profiles, err := NewUserProfileModel(conn).SearchPublicPage(ctx, "go", 0, 10)
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, []int64{2, 1}, []int64{profiles[0].Id, profiles[1].Id})
}
