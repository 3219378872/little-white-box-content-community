//go:build integration

package logic

import (
	"context"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetUserPosts_SortBy 验证 GetUserPosts 在 sortBy=最新 / sortBy=热门 下的顺序。
// 构造 3 个帖子（不同 created_at、like_count），分别请求两种排序并比对顺序。
func TestGetUserPosts_SortBy(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in short mode")
	}

	ctx := context.Background()
	const authorID int64 = 9001

	// 三个帖子：A 最早、点赞 10；B 居中、点赞 50（热门第一）；C 最新、点赞 0
	idA := createTestPost(t, authorID, "A", "内容 A", []string{"golang"})
	idB := createTestPost(t, authorID, "B", "内容 B", []string{"golang"})
	idC := createTestPost(t, authorID, "C", "内容 C", []string{"golang"})

	// 调整 created_at 与 like_count 以制造可区分顺序
	now := time.Now()
	_, err := testSvcCtx.Conn.ExecCtx(ctx,
		"UPDATE `post` SET `created_at`=?, `like_count`=? WHERE `id`=?",
		now.Add(-3*time.Hour), 10, idA)
	require.NoError(t, err)
	_, err = testSvcCtx.Conn.ExecCtx(ctx,
		"UPDATE `post` SET `created_at`=?, `like_count`=? WHERE `id`=?",
		now.Add(-2*time.Hour), 50, idB)
	require.NoError(t, err)
	_, err = testSvcCtx.Conn.ExecCtx(ctx,
		"UPDATE `post` SET `created_at`=?, `like_count`=? WHERE `id`=?",
		now.Add(-1*time.Hour), 0, idC)
	require.NoError(t, err)

	t.Run("sortBy=1 最新优先", func(t *testing.T) {
		l := NewGetUserPostsLogic(ctx, testSvcCtx)
		resp, err := l.GetUserPosts(&pb.GetUserPostsReq{
			UserId: authorID, Page: 1, PageSize: 10, SortBy: 1,
		})
		require.NoError(t, err)
		require.Len(t, resp.Posts, 3)
		assert.Equal(t, int64(3), resp.Total)
		assert.Equal(t, idC, resp.Posts[0].Id, "最新创建的 C 应在首位")
		assert.Equal(t, idB, resp.Posts[1].Id)
		assert.Equal(t, idA, resp.Posts[2].Id)
	})

	t.Run("sortBy=2 热门优先", func(t *testing.T) {
		l := NewGetUserPostsLogic(ctx, testSvcCtx)
		resp, err := l.GetUserPosts(&pb.GetUserPostsReq{
			UserId: authorID, Page: 1, PageSize: 10, SortBy: 2,
		})
		require.NoError(t, err)
		require.Len(t, resp.Posts, 3)
		assert.Equal(t, idB, resp.Posts[0].Id, "like_count 最高的 B 应在首位")
		assert.Equal(t, idA, resp.Posts[1].Id, "like_count 次高的 A 在第二")
		assert.Equal(t, idC, resp.Posts[2].Id, "like_count 最低的 C 在末位")
	})

	t.Run("sortBy 非法值回退最新", func(t *testing.T) {
		l := NewGetUserPostsLogic(ctx, testSvcCtx)
		resp, err := l.GetUserPosts(&pb.GetUserPostsReq{
			UserId: authorID, Page: 1, PageSize: 10, SortBy: 99,
		})
		require.NoError(t, err)
		require.Len(t, resp.Posts, 3)
		assert.Equal(t, idC, resp.Posts[0].Id)
	})
}
