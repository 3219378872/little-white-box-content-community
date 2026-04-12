//go:build integration

package logic

import (
	"context"
	"esx/app/content/pb/xiaobaihe/content/pb"
	"testing"

	"errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePost(t *testing.T) {
	ctx := context.Background()

	t.Run("成功创建帖子", func(t *testing.T) {
		l := NewCreatePostLogic(ctx, testSvcCtx)
		resp, err := l.CreatePost(&pb.CreatePostReq{
			AuthorId: 1001,
			Title:    "测试标题",
			Content:  "测试内容",
			Tags:     []string{"golang", "rust"},
			Status:   1,
		})
		require.NoError(t, err)
		assert.Greater(t, resp.PostId, int64(0))

		// 验证帖子内容正确写入
		gl := NewGetPostLogic(ctx, testSvcCtx)
		getResp, err := gl.GetPost(&pb.GetPostReq{PostId: resp.PostId})
		require.NoError(t, err)
		assert.Equal(t, "测试标题", getResp.Post.Title)
		assert.Equal(t, "测试内容", getResp.Post.Content)
		assert.Equal(t, int64(1001), getResp.Post.AuthorId)
		assert.ElementsMatch(t, []string{"golang", "rust"}, getResp.Post.Tags)
	})

	t.Run("空标题报错", func(t *testing.T) {
		l := NewCreatePostLogic(ctx, testSvcCtx)
		_, err := l.CreatePost(&pb.CreatePostReq{
			AuthorId: 1001,
			Title:    "",
			Content:  "内容",
		})
		assertBizError(t, err, errx.TitleEmpty)
	})

	t.Run("空内容报错", func(t *testing.T) {
		l := NewCreatePostLogic(ctx, testSvcCtx)
		_, err := l.CreatePost(&pb.CreatePostReq{
			AuthorId: 1001,
			Title:    "标题",
			Content:  "",
		})
		assertBizError(t, err, errx.ContentEmpty)
	})

	t.Run("图片URL含逗号报错", func(t *testing.T) {
		l := NewCreatePostLogic(ctx, testSvcCtx)
		_, err := l.CreatePost(&pb.CreatePostReq{
			AuthorId: 1001,
			Title:    "标题",
			Content:  "内容",
			Images:   []string{"http://example.com/a,b.jpg"},
		})
		assertBizError(t, err, errx.ParamError)
	})
}

func TestGetPost(t *testing.T) {
	ctx := context.Background()

	t.Run("成功获取帖子", func(t *testing.T) {
		postId := createTestPost(t, 1002, "获取测试", "内容", []string{"golang"})

		l := NewGetPostLogic(ctx, testSvcCtx)
		resp, err := l.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, postId, resp.Post.Id)
		assert.Equal(t, "获取测试", resp.Post.Title)
		assert.Equal(t, int64(1002), resp.Post.AuthorId)
		assert.Equal(t, int32(1), resp.Post.Status)
		assert.Contains(t, resp.Post.Tags, "golang")
	})

	t.Run("帖子不存在报错", func(t *testing.T) {
		l := NewGetPostLogic(ctx, testSvcCtx)
		_, err := l.GetPost(&pb.GetPostReq{PostId: 999999999})
		assertBizError(t, err, errx.ContentNotFound)
	})

	t.Run("已删除帖子报错", func(t *testing.T) {
		postId := createTestPost(t, 1003, "待删除帖子", "内容", nil)

		// 软删除
		dl := NewDeletePostLogic(ctx, testSvcCtx)
		_, err := dl.DeletePost(&pb.DeletePostReq{PostId: postId, AuthorId: 1003})
		require.NoError(t, err)

		// 获取已删除帖子
		gl := NewGetPostLogic(ctx, testSvcCtx)
		_, err = gl.GetPost(&pb.GetPostReq{PostId: postId})
		assertBizError(t, err, errx.PostAlreadyDeleted)
	})
}

func TestUpdatePost(t *testing.T) {
	ctx := context.Background()

	t.Run("成功更新帖子", func(t *testing.T) {
		postId := createTestPost(t, 2001, "原标题", "原内容", []string{"golang"})

		l := NewUpdatePostLogic(ctx, testSvcCtx)
		_, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId:   postId,
			AuthorId: 2001,
			Title:    "新标题",
			Content:  "新内容",
			Tags:     []string{"rust", "python"},
			Status:   1,
		})
		require.NoError(t, err)

		// 验证更新后内容
		gl := NewGetPostLogic(ctx, testSvcCtx)
		resp, err := gl.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, "新标题", resp.Post.Title)
		assert.Equal(t, "新内容", resp.Post.Content)
		assert.ElementsMatch(t, []string{"rust", "python"}, resp.Post.Tags)
	})

	t.Run("帖子不存在报错", func(t *testing.T) {
		l := NewUpdatePostLogic(ctx, testSvcCtx)
		_, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId:   999999999,
			AuthorId: 2001,
			Title:    "标题",
			Content:  "内容",
		})
		assertBizError(t, err, errx.ContentNotFound)
	})

	t.Run("非作者操作报错", func(t *testing.T) {
		postId := createTestPost(t, 2002, "标题", "内容", nil)

		l := NewUpdatePostLogic(ctx, testSvcCtx)
		_, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId:   postId,
			AuthorId: 2003, // 不同的用户
			Title:    "标题",
			Content:  "内容",
		})
		assertBizError(t, err, errx.ContentForbidden)
	})

	t.Run("更新已删除帖子报错", func(t *testing.T) {
		postId := createTestPost(t, 2004, "标题", "内容", nil)

		dl := NewDeletePostLogic(ctx, testSvcCtx)
		_, err := dl.DeletePost(&pb.DeletePostReq{PostId: postId, AuthorId: 2004})
		require.NoError(t, err)

		l := NewUpdatePostLogic(ctx, testSvcCtx)
		_, err = l.UpdatePost(&pb.UpdatePostReq{
			PostId:   postId,
			AuthorId: 2004,
			Title:    "标题",
			Content:  "内容",
		})
		assertBizError(t, err, errx.PostAlreadyDeleted)
	})
}

func TestDeletePost(t *testing.T) {
	ctx := context.Background()

	t.Run("成功删除帖子", func(t *testing.T) {
		postId := createTestPost(t, 3001, "标题", "内容", nil)

		l := NewDeletePostLogic(ctx, testSvcCtx)
		_, err := l.DeletePost(&pb.DeletePostReq{PostId: postId, AuthorId: 3001})
		require.NoError(t, err)

		// 再次获取应返回已删除错误
		gl := NewGetPostLogic(ctx, testSvcCtx)
		_, err = gl.GetPost(&pb.GetPostReq{PostId: postId})
		assertBizError(t, err, errx.PostAlreadyDeleted)
	})

	t.Run("帖子不存在报错", func(t *testing.T) {
		l := NewDeletePostLogic(ctx, testSvcCtx)
		_, err := l.DeletePost(&pb.DeletePostReq{PostId: 999999999, AuthorId: 3001})
		assertBizError(t, err, errx.ContentNotFound)
	})

	t.Run("非作者删除报错", func(t *testing.T) {
		postId := createTestPost(t, 3002, "标题", "内容", nil)

		l := NewDeletePostLogic(ctx, testSvcCtx)
		_, err := l.DeletePost(&pb.DeletePostReq{PostId: postId, AuthorId: 3003})
		assertBizError(t, err, errx.ContentForbidden)
	})
}

func TestGetPostList(t *testing.T) {
	ctx := context.Background()

	// 创建3篇帖子用于列表测试
	createTestPost(t, 4001, "列表帖子1", "内容1", nil)
	createTestPost(t, 4001, "列表帖子2", "内容2", nil)
	createTestPost(t, 4001, "列表帖子3", "内容3", nil)

	t.Run("正常获取列表", func(t *testing.T) {
		l := NewGetPostListLogic(ctx, testSvcCtx)
		resp, err := l.GetPostList(&pb.GetPostListReq{
			Page:     1,
			PageSize: 10,
			SortBy:   1,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(resp.Posts), 3)
		assert.GreaterOrEqual(t, resp.Total, int64(3))
	})

	t.Run("分页截断", func(t *testing.T) {
		l := NewGetPostListLogic(ctx, testSvcCtx)
		resp, err := l.GetPostList(&pb.GetPostListReq{
			Page:     1,
			PageSize: 2,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(resp.Posts))
		assert.GreaterOrEqual(t, resp.Total, int64(3))
	})
}

func TestGetUserPosts(t *testing.T) {
	ctx := context.Background()

	t.Run("获取用户帖子列表", func(t *testing.T) {
		createTestPost(t, 5001, "用户帖子1", "内容1", nil)
		createTestPost(t, 5001, "用户帖子2", "内容2", nil)

		l := NewGetUserPostsLogic(ctx, testSvcCtx)
		resp, err := l.GetUserPosts(&pb.GetUserPostsReq{
			UserId:   5001,
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(resp.Posts))
		assert.Equal(t, int64(2), resp.Total)
		for _, p := range resp.Posts {
			assert.Equal(t, int64(5001), p.AuthorId)
		}
	})

	t.Run("无帖子用户返回空列表", func(t *testing.T) {
		l := NewGetUserPostsLogic(ctx, testSvcCtx)
		resp, err := l.GetUserPosts(&pb.GetUserPostsReq{
			UserId:   5999,
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, len(resp.Posts))
		assert.Equal(t, int64(0), resp.Total)
	})
}
