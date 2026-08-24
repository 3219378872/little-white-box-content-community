//go:build integration

package logic

import (
	"context"
	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"testing"

	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePostIdempotency(t *testing.T) {
	ctx := context.Background()

	t.Run("同一幂等键同命令重试返回原帖子ID且不重复创建", func(t *testing.T) {
		l := NewCreatePostLogic(ctx, testSvcCtx)
		req := &pb.CreatePostReq{
			AuthorId: 9101, Title: "幂等标题", Content: "幂等内容",
			Status: 1, IdempotencyKey: "idem-post-1",
		}
		first, err := l.CreatePost(req)
		require.NoError(t, err)

		second, err := l.CreatePost(req)
		require.NoError(t, err)
		assert.Equal(t, first.PostId, second.PostId)
		assert.Equal(t, int64(1), second.Revision)

		list, err := NewGetUserPostsLogic(ctx, testSvcCtx).GetUserPosts(&pb.GetUserPostsReq{
			UserId: 9101, PageSize: 10,
		})
		require.NoError(t, err)
		count := 0
		for _, p := range list.Posts {
			if p.Id == first.PostId {
				count++
			}
		}
		assert.Equal(t, 1, count, "同一幂等键不得产生重复帖子")
	})

	t.Run("同一幂等键不同命令返回冲突", func(t *testing.T) {
		l := NewCreatePostLogic(ctx, testSvcCtx)
		_, err := l.CreatePost(&pb.CreatePostReq{
			AuthorId: 9102, Title: "冲突标题", Content: "冲突内容",
			Status: 1, IdempotencyKey: "idem-post-conflict",
		})
		require.NoError(t, err)

		_, err = l.CreatePost(&pb.CreatePostReq{
			AuthorId: 9102, Title: "不同标题", Content: "不同内容",
			Status: 1, IdempotencyKey: "idem-post-conflict",
		})
		assertBizError(t, err, errx.IdempotencyConflict)
	})
}

func TestPostRevisionConflict(t *testing.T) {
	ctx := context.Background()

	t.Run("更新携带过期revision返回409", func(t *testing.T) {
		postId := createTestPost(t, 9103, "版本标题", "版本内容", nil)

		l := NewUpdatePostLogic(ctx, testSvcCtx)
		_, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9103,
			Title: "新标题", Content: "新内容", ExpectedRevision: 99,
		})
		assertBizError(t, err, errx.ContentVersionConflict)
	})

	t.Run("更新成功递增revision并返回新值", func(t *testing.T) {
		postId := createTestPost(t, 9104, "版本标题2", "版本内容2", nil)

		l := NewUpdatePostLogic(ctx, testSvcCtx)
		resp, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9104,
			Title: "新标题2", Content: "新内容2", ExpectedRevision: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.Revision)

		get, err := NewGetPostLogic(ctx, testSvcCtx).GetPost(&pb.GetPostReq{PostId: postId, UserId: 9104})
		require.NoError(t, err)
		assert.Equal(t, int64(2), get.Post.Revision)
		assert.Equal(t, "新标题2", get.Post.Title)
	})

	t.Run("删除携带过期revision返回409", func(t *testing.T) {
		postId := createTestPost(t, 9105, "删除版本", "内容", nil)

		_, err := NewDeletePostLogic(ctx, testSvcCtx).DeletePost(&pb.DeletePostReq{
			PostId: postId, AuthorId: 9105, ExpectedRevision: 99,
		})
		assertBizError(t, err, errx.ContentVersionConflict)
	})

	t.Run("迁移期旧客户端不带revision可更新并递增revision", func(t *testing.T) {
		postId := createTestPost(t, 9109, "迁移版本", "内容", nil)

		l := NewUpdatePostLogic(ctx, testSvcCtx)
		resp, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9109,
			Title: "新标题", Content: "新内容",
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), resp.Revision)
	})

	t.Run("迁移期旧客户端不带revision可删除", func(t *testing.T) {
		postId := createTestPost(t, 9110, "迁移删除", "内容", nil)

		_, err := NewDeletePostLogic(ctx, testSvcCtx).DeletePost(&pb.DeletePostReq{
			PostId: postId, AuthorId: 9110,
		})
		require.NoError(t, err)
	})
}

func TestPostStatusTransitions(t *testing.T) {
	ctx := context.Background()

	t.Run("发布->草稿->发布 双向转换", func(t *testing.T) {
		postId := createTestPost(t, 9106, "状态标题", "状态内容", nil)

		// published -> draft（取消发布）
		l := NewUpdatePostLogic(ctx, testSvcCtx)
		resp, err := l.UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9106,
			Title: "状态标题", Content: "状态内容",
			Status: int32Ptr(0), ExpectedRevision: 1,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(0), resp.Status)
		assert.Equal(t, int64(2), resp.Revision)

		// 非作者读草稿应统一不存在
		_, err = NewGetPostLogic(ctx, testSvcCtx).GetPost(&pb.GetPostReq{PostId: postId, UserId: 999})
		assertBizError(t, err, errx.ContentNotFound)

		// 作者可读草稿
		get, err := NewGetPostLogic(ctx, testSvcCtx).GetPost(&pb.GetPostReq{PostId: postId, UserId: 9106})
		require.NoError(t, err)
		assert.Equal(t, int32(0), get.Post.Status)

		// draft -> published（重新发布）
		resp, err = l.UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9106,
			Title: "状态标题", Content: "状态内容",
			Status: int32Ptr(1), ExpectedRevision: 2,
		})
		require.NoError(t, err)
		assert.Equal(t, int32(1), resp.Status)
		assert.Equal(t, int64(3), resp.Revision)
	})
}

func TestCreateCommentIdempotency(t *testing.T) {
	ctx := context.Background()
	postId := createTestPost(t, 9107, "评论幂等", "内容", nil)

	l := NewCreateCommentLogic(ctx, testSvcCtx)
	req := &pb.CreateCommentReq{
		PostId: postId, UserId: 9108, Content: "幂等评论",
		IdempotencyKey: "idem-comment-1",
	}
	first, err := l.CreateComment(req)
	require.NoError(t, err)

	second, err := l.CreateComment(req)
	require.NoError(t, err)
	assert.Equal(t, first.CommentId, second.CommentId)
}
