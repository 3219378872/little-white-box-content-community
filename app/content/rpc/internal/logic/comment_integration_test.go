//go:build integration

package logic

import (
	"context"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"fmt"
	"testing"

	"esx/pkg/errx"

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

	t.Run("创建回复后父评论 reply_count 递增", func(t *testing.T) {
		postId := createTestPost(t, 6101, "计数测试帖子", "内容", nil)
		parentId := createTestComment(t, postId, 6101, "顶级评论")

		createTestReply(t, postId, 6102, parentId, 6101, "回复1")
		createTestReply(t, postId, 6103, parentId, 6101, "回复2")

		parent, err := testSvcCtx.CommentModel.FindCommentById(ctx, parentId)
		require.NoError(t, err)
		assert.Equal(t, int64(2), parent.ReplyCount)

		gl := NewGetPostLogic(ctx, testSvcCtx)
		postResp, err := gl.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, int64(3), postResp.Post.CommentCount)
	})

	t.Run("回复缺少被回复用户报错", func(t *testing.T) {
		postId := createTestPost(t, 6104, "帖子", "内容", nil)
		parentId := createTestComment(t, postId, 6104, "顶级评论")

		l := NewCreateCommentLogic(ctx, testSvcCtx)
		_, err := l.CreateComment(&pb.CreateCommentReq{
			PostId:   postId,
			UserId:   6105,
			ParentId: parentId,
			Content:  "只有父评论没有被回复用户",
		})
		assertBizError(t, err, errx.ParamError)
	})

	t.Run("对楼中楼再回复报错", func(t *testing.T) {
		postId := createTestPost(t, 6106, "帖子", "内容", nil)
		parentId := createTestComment(t, postId, 6106, "顶级评论")
		replyId := createTestReply(t, postId, 6107, parentId, 6106, "一级回复")

		l := NewCreateCommentLogic(ctx, testSvcCtx)
		_, err := l.CreateComment(&pb.CreateCommentReq{
			PostId:      postId,
			UserId:      6108,
			ParentId:    replyId,
			ReplyUserId: 6107,
			Content:     "对回复的回复",
		})
		assertBizError(t, err, errx.ParamError)
	})

	t.Run("对已删除评论回复报错", func(t *testing.T) {
		postId := createTestPost(t, 6109, "帖子", "内容", nil)
		parentId := createTestComment(t, postId, 6109, "顶级评论")

		dl := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := dl.DeleteComment(&pb.DeleteCommentReq{CommentId: parentId, UserId: 6109})
		require.NoError(t, err)

		cl := NewCreateCommentLogic(ctx, testSvcCtx)
		_, err = cl.CreateComment(&pb.CreateCommentReq{
			PostId:      postId,
			UserId:      6110,
			ParentId:    parentId,
			ReplyUserId: 6109,
			Content:     "回复已删评论",
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

	t.Run("删除子回复回减父评论 reply_count", func(t *testing.T) {
		postId := createTestPost(t, 7101, "帖子", "内容", nil)
		parentId := createTestComment(t, postId, 7101, "顶级评论")
		replyId := createTestReply(t, postId, 7102, parentId, 7101, "回复1")

		l := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := l.DeleteComment(&pb.DeleteCommentReq{CommentId: replyId, UserId: 7102})
		require.NoError(t, err)

		parent, err := testSvcCtx.CommentModel.FindCommentById(ctx, parentId)
		require.NoError(t, err)
		assert.Equal(t, int64(0), parent.ReplyCount)

		gl := NewGetPostLogic(ctx, testSvcCtx)
		postResp, err := gl.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, int64(1), postResp.Post.CommentCount)
	})

	t.Run("删除顶级评论级联软删全部子回复并修正帖子计数", func(t *testing.T) {
		postId := createTestPost(t, 7201, "帖子", "内容", nil)
		parentId := createTestComment(t, postId, 7201, "顶级评论")
		createTestReply(t, postId, 7202, parentId, 7201, "回复1")
		createTestReply(t, postId, 7203, parentId, 7201, "回复2")
		createTestReply(t, postId, 7204, parentId, 7201, "回复3")

		l := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := l.DeleteComment(&pb.DeleteCommentReq{CommentId: parentId, UserId: 7201})
		require.NoError(t, err)

		parent, err := testSvcCtx.CommentModel.FindCommentById(ctx, parentId)
		require.NoError(t, err)
		assert.Equal(t, int64(0), parent.Status)

		replies, total, err := testSvcCtx.CommentModel.FindByParentId(ctx, parentId, 1, 20)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Equal(t, 0, len(replies))

		gl := NewGetPostLogic(ctx, testSvcCtx)
		postResp, err := gl.GetPost(&pb.GetPostReq{PostId: postId})
		require.NoError(t, err)
		assert.Equal(t, int64(0), postResp.Post.CommentCount)
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
		createTestReply(t, postId, 8003, parentId, 8002, "回复评论")

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

	t.Run("内嵌回复预览与回复总数", func(t *testing.T) {
		postId := createTestPost(t, 8101, "帖子", "内容", nil)
		parentA := createTestComment(t, postId, 8101, "父评论A")
		for _, content := range []string{"回1", "回2", "回3", "回4", "回5"} {
			createTestReply(t, postId, 8102, parentA, 8101, content)
		}
		parentB := createTestComment(t, postId, 8101, "父评论B")

		l := NewGetCommentListLogic(ctx, testSvcCtx)
		resp, err := l.GetCommentList(&pb.GetCommentListReq{
			PostId:   postId,
			Page:     1,
			PageSize: 10,
			SortBy:   1,
		})
		require.NoError(t, err)

		byId := map[int64]*pb.CommentInfo{}
		for _, c := range resp.Comments {
			byId[c.Id] = c
		}
		threadA := byId[parentA]
		require.NotNil(t, threadA)
		assert.Equal(t, int64(5), threadA.ReplyCount)
		require.Len(t, threadA.Replies, 3) // 只内嵌前 3 条预览
		assert.Equal(t, "回1", threadA.Replies[0].Content)
		assert.Equal(t, "回3", threadA.Replies[2].Content)
		assert.Empty(t, threadA.Replies[0].Replies)

		threadB := byId[parentB]
		require.NotNil(t, threadB)
		assert.Equal(t, int64(0), threadB.ReplyCount)
		assert.Empty(t, threadB.Replies)
	})
}

func TestGetCommentReplies(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T, authorId int64) (int64, int64) {
		postId := createTestPost(t, authorId, "楼中楼帖子", "内容", nil)
		parentId := createTestComment(t, postId, authorId, "顶级评论")
		for i := int64(1); i <= 5; i++ {
			createTestReply(t, postId, authorId+i, parentId, authorId,
				fmt.Sprintf("回复%d", i))
		}
		return postId, parentId
	}

	t.Run("全量分页时间正序", func(t *testing.T) {
		postId, parentId := setup(t, 9501)
		_ = postId

		l := NewGetCommentRepliesLogic(ctx, testSvcCtx)
		resp, err := l.GetCommentReplies(&pb.GetCommentRepliesReq{
			CommentId: parentId,
			Page:      1,
			PageSize:  3,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(5), resp.Total)
		require.Len(t, resp.Comments, 3)
		assert.Equal(t, "回复1", resp.Comments[0].Content)
		assert.Equal(t, "回复3", resp.Comments[2].Content)

		page2, err := l.GetCommentReplies(&pb.GetCommentRepliesReq{
			CommentId: parentId,
			Page:      2,
			PageSize:  3,
		})
		require.NoError(t, err)
		require.Len(t, page2.Comments, 2)
		assert.Equal(t, "回复4", page2.Comments[0].Content)
	})

	t.Run("父评论不存在报错", func(t *testing.T) {
		l := NewGetCommentRepliesLogic(ctx, testSvcCtx)
		_, err := l.GetCommentReplies(&pb.GetCommentRepliesReq{CommentId: 999999999})
		assertBizError(t, err, errx.ContentNotFound)
	})

	t.Run("已删除父评论的回复统一不存在", func(t *testing.T) {
		postId, parentId := setup(t, 9601)

		dl := NewDeleteCommentLogic(ctx, testSvcCtx)
		_, err := dl.DeleteComment(&pb.DeleteCommentReq{CommentId: parentId, UserId: 9601})
		require.NoError(t, err)

		l := NewGetCommentRepliesLogic(ctx, testSvcCtx)
		_, err = l.GetCommentReplies(&pb.GetCommentRepliesReq{CommentId: parentId})
		assertBizError(t, err, errx.ContentNotFound)
		_ = postId
	})

	t.Run("草稿帖的回复统一不存在", func(t *testing.T) {
		postId, parentId := setup(t, 9701)

		pl := NewUpdatePostLogic(ctx, testSvcCtx)
		_, err := pl.UpdatePost(&pb.UpdatePostReq{
			PostId:           postId,
			AuthorId:         9701,
			Title:            "楼中楼帖子",
			Content:          "内容",
			Status:           int32Ptr(0),
			ExpectedRevision: 1,
		})
		require.NoError(t, err)

		l := NewGetCommentRepliesLogic(ctx, testSvcCtx)
		_, err = l.GetCommentReplies(&pb.GetCommentRepliesReq{CommentId: parentId})
		assertBizError(t, err, errx.ContentNotFound)
	})
}
