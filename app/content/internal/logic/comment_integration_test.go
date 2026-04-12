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

func TestCreateComment(t *testing.T) {
	ctx := context.Background()

	t.Run("成功创建顶级评论", func(t *testing.T) {
		postId := createTestPost(t, 6001, "评论测试帖子", "内容", nil)

		l := NewCreateCommentLogic(ctx, testSvcCtx)
		resp, err := l.CreateComment(&pb.CreateCommentReq{
			PostId:  postId,
			UserId:  6001,
			Content: "这是顶级评论",
		})
		require.NoError(t, err)
		assert.Greater(t, resp.CommentId, int64(0))

		// 验证帖子评论数递增
		gl := NewGetPostLogic(ctx, testSvcCtx)
		postResp, err := gl.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, int64(1), postResp.Post.CommentCount)
	})

	t.Run("成功创建回复评论", func(t *testing.T) {
		postId := createTestPost(t, 6002, "回复测试帖子", "内容", nil)
		parentCommentId := createTestComment(t, postId, 6002, "顶级评论")

		l := NewCreateCommentLogic(ctx, testSvcCtx)
		resp, err := l.CreateComment(&pb.CreateCommentReq{
			PostId:      postId,
			UserId:      6003,
			ParentId:    parentCommentId,
			ReplyUserId: 6002,
			Content:     "这是回复评论",
		})
		require.NoError(t, err)
		assert.Greater(t, resp.CommentId, int64(0))
	})

	t.Run("空内容报错", func(t *testing.T) {
		postId := createTestPost(t, 6004, "帖子", "内容", nil)

		l := NewCreateCommentLogic(ctx, testSvcCtx)
		_, err := l.CreateComment(&pb.CreateCommentReq{
			PostId:  postId,
			UserId:  6004,
			Content: "",
		})
		assertBizError(t, err, errx.ContentEmpty)
	})

	t.Run("帖子不存在报错", func(t *testing.T) {
		l := NewCreateCommentLogic(ctx, testSvcCtx)
		_, err := l.CreateComment(&pb.CreateCommentReq{
			PostId:  999999999,
			UserId:  6005,
			Content: "评论内容",
		})
		assertBizError(t, err, errx.ContentNotFound)
	})
}

func TestDeleteComment(t *testing.T) {
	ctx := context.Background()

	t.Run("成功删除评论", func(t *testing.T) {
		postId := createTestPost(t, 7001, "帖子", "内容", nil)
		commentId := createTestComment(t, postId, 7001, "待删除评论")

		l := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := l.DeleteComment(&pb.DeleteCommentReq{
			CommentId: commentId,
			UserId:    7001,
		})
		require.NoError(t, err)

		// 验证帖子评论数递减
		gl := NewGetPostLogic(ctx, testSvcCtx)
		postResp, err := gl.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, int64(0), postResp.Post.CommentCount)
	})

	t.Run("重复删除幂等", func(t *testing.T) {
		postId := createTestPost(t, 7002, "帖子", "内容", nil)
		commentId := createTestComment(t, postId, 7002, "评论")

		l := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := l.DeleteComment(&pb.DeleteCommentReq{CommentId: commentId, UserId: 7002})
		require.NoError(t, err)

		// 再次删除不报错
		_, err = l.DeleteComment(&pb.DeleteCommentReq{CommentId: commentId, UserId: 7002})
		require.NoError(t, err)
	})

	t.Run("评论不存在报错", func(t *testing.T) {
		l := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := l.DeleteComment(&pb.DeleteCommentReq{
			CommentId: 999999999,
			UserId:    7001,
		})
		assertBizError(t, err, errx.ContentNotFound)
	})

	t.Run("非作者删除报错", func(t *testing.T) {
		postId := createTestPost(t, 7003, "帖子", "内容", nil)
		commentId := createTestComment(t, postId, 7003, "评论")

		l := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := l.DeleteComment(&pb.DeleteCommentReq{
			CommentId: commentId,
			UserId:    7004, // 不同用户
		})
		assertBizError(t, err, errx.ContentForbidden)
	})
}

func TestGetCommentList(t *testing.T) {
	ctx := context.Background()

	t.Run("获取顶级评论列表", func(t *testing.T) {
		postId := createTestPost(t, 8001, "帖子", "内容", nil)
		createTestComment(t, postId, 8001, "评论1")
		createTestComment(t, postId, 8001, "评论2")
		createTestComment(t, postId, 8001, "评论3")

		l := NewGetCommentListLogic(ctx, testSvcCtx)
		resp, err := l.GetCommentList(&pb.GetCommentListReq{
			PostId:   postId,
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 3, len(resp.Comments))
		assert.Equal(t, int64(3), resp.Total)
	})

	t.Run("回复评论不在顶级列表中", func(t *testing.T) {
		postId := createTestPost(t, 8002, "帖子", "内容", nil)
		parentId := createTestComment(t, postId, 8002, "顶级评论")

		// 创建一条回复评论
		cl := NewCreateCommentLogic(ctx, testSvcCtx)
		_, err := cl.CreateComment(&pb.CreateCommentReq{
			PostId:   postId,
			UserId:   8003,
			ParentId: parentId,
			Content:  "回复评论",
		})
		require.NoError(t, err)

		l := NewGetCommentListLogic(ctx, testSvcCtx)
		resp, err := l.GetCommentList(&pb.GetCommentListReq{
			PostId:   postId,
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		// 只返回顶级评论（parent_id IS NULL）
		assert.Equal(t, 1, len(resp.Comments))
		assert.Equal(t, int64(1), resp.Total)
	})

	t.Run("无评论帖子返回空列表", func(t *testing.T) {
		postId := createTestPost(t, 8004, "帖子", "内容", nil)

		l := NewGetCommentListLogic(ctx, testSvcCtx)
		resp, err := l.GetCommentList(&pb.GetCommentListReq{
			PostId:   postId,
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, len(resp.Comments))
		assert.Equal(t, int64(0), resp.Total)
	})
}
