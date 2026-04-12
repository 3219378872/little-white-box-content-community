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

func TestGetTags(t *testing.T) {
	ctx := context.Background()

	t.Run("默认Limit返回标签列表", func(t *testing.T) {
		l := NewGetTagsLogic(ctx, testSvcCtx)
		resp, err := l.GetTags(&pb.GetTagsReq{Limit: 0})
		require.NoError(t, err)
		// TestMain 已预置 3 条标签
		assert.Equal(t, 3, len(resp.Tags))
		// 按 post_count 降序：golang(100) > python(80) > rust(50)
		assert.Equal(t, "golang", resp.Tags[0].Name)
		assert.Equal(t, int64(100), resp.Tags[0].PostCount)
	})

	t.Run("自定义Limit截断结果", func(t *testing.T) {
		l := NewGetTagsLogic(ctx, testSvcCtx)
		resp, err := l.GetTags(&pb.GetTagsReq{Limit: 2})
		require.NoError(t, err)
		assert.Equal(t, 2, len(resp.Tags))
	})
}

func TestGetPostsByTag(t *testing.T) {
	ctx := context.Background()

	t.Run("根据标签名获取帖子", func(t *testing.T) {
		createTestPost(t, 9001, "标签帖子1", "内容1", []string{"integration-test-tag"})
		createTestPost(t, 9001, "标签帖子2", "内容2", []string{"integration-test-tag"})

		l := NewGetPostsByTagLogic(ctx, testSvcCtx)
		resp, err := l.GetPostsByTag(&pb.GetPostsByTagReq{
			TagName:  "integration-test-tag",
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, len(resp.Posts))
		assert.Equal(t, int64(2), resp.Total)
		for _, p := range resp.Posts {
			assert.Contains(t, p.Tags, "integration-test-tag")
		}
	})

	t.Run("空标签名报错", func(t *testing.T) {
		l := NewGetPostsByTagLogic(ctx, testSvcCtx)
		_, err := l.GetPostsByTag(&pb.GetPostsByTagReq{TagName: ""})
		assertBizError(t, err, errx.ParamError)
	})

	t.Run("无匹配帖子返回空列表", func(t *testing.T) {
		l := NewGetPostsByTagLogic(ctx, testSvcCtx)
		resp, err := l.GetPostsByTag(&pb.GetPostsByTagReq{
			TagName:  "nonexistent-tag-xyz",
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, len(resp.Posts))
		assert.Equal(t, int64(0), resp.Total)
	})
}
