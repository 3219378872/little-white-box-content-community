//go:build integration

package logic

import (
	"context"
	"strings"
	"testing"

	"esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B3：.api 声明 UpdatePostV2 的 title/content 为 optional，实现须按局部更新
// 语义把请求字段合并到现值后再校验；此前实现无条件要求两字段非空，
// 与 .api 及网关决策表（title-only 合法用例）相悖（黑盒 e2e 首轮发现）。

func TestUpdatePostTitleOnlyKeepsContent(t *testing.T) {
	ctx := context.Background()
	postId := createTestPost(t, 9301, "原标题", "原内容保持不变", nil)

	resp, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
		PostId: postId, AuthorId: 9301,
		Title: "只改标题", ExpectedRevision: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Revision)
	assert.Equal(t, int32(1), resp.Status)

	get, err := NewGetPostLogic(ctx, testSvcCtx).GetPost(&pb.GetPostReq{PostId: postId, UserId: 9301})
	require.NoError(t, err)
	assert.Equal(t, "只改标题", get.Post.Title)
	assert.Equal(t, "原内容保持不变", get.Post.Content)
}

func TestUpdatePostContentOnlyKeepsTitleAndTags(t *testing.T) {
	ctx := context.Background()
	postId := createTestPost(t, 9302, "标题保持", "旧内容", []string{"golang", "rust"})

	resp, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
		PostId: postId, AuthorId: 9302,
		Content: "只改内容，标签必须保留", ExpectedRevision: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), resp.Revision)

	get, err := NewGetPostLogic(ctx, testSvcCtx).GetPost(&pb.GetPostReq{PostId: postId, UserId: 9302})
	require.NoError(t, err)
	assert.Equal(t, "标题保持", get.Post.Title)
	assert.Equal(t, "只改内容，标签必须保留", get.Post.Content)
	assert.ElementsMatch(t, []string{"golang", "rust"}, get.Post.Tags)
}

func TestUpdatePostExplicitTagsReplaceWhileTitleOnlyPreservesThem(t *testing.T) {
	ctx := context.Background()
	postId := createTestPost(t, 9303, "标签语义", "内容", []string{"old-a", "old-b"})

	_, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
		PostId: postId, AuthorId: 9303,
		Title: "带新标签的更新", Tags: []string{"new-tag"}, ExpectedRevision: 1,
	})
	require.NoError(t, err)

	get, err := NewGetPostLogic(ctx, testSvcCtx).GetPost(&pb.GetPostReq{PostId: postId, UserId: 9303})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"new-tag"}, get.Post.Tags)
}

func TestUpdatePostEmptyPayloadRejected(t *testing.T) {
	ctx := context.Background()
	postId := createTestPost(t, 9304, "空载荷", "内容", nil)

	_, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
		PostId: postId, AuthorId: 9304, ExpectedRevision: 1,
	})
	assertBizError(t, err, errx.ParamError)
}

func TestUpdatePostValidatesMergedLengths(t *testing.T) {
	ctx := context.Background()

	t.Run("合并后的内容超长返回ContentTooLong", func(t *testing.T) {
		postId := createTestPost(t, 9305, "超长校验", "短内容", nil)
		_, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9305,
			Content: strings.Repeat("长", 20001), ExpectedRevision: 1,
		})
		assertBizError(t, err, errx.ContentTooLong)
	})

	t.Run("合并后的标题超长返回ContentTooLong", func(t *testing.T) {
		postId := createTestPost(t, 9306, "短标题", "内容", nil)
		_, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9306,
			Title: strings.Repeat("标", 121), ExpectedRevision: 1,
		})
		assertBizError(t, err, errx.ContentTooLong)
	})

	t.Run("content-only更新沿用原标题不触发TitleEmpty", func(t *testing.T) {
		postId := createTestPost(t, 9307, "仅改内容", "旧", nil)
		_, err := NewUpdatePostLogic(ctx, testSvcCtx).UpdatePost(&pb.UpdatePostReq{
			PostId: postId, AuthorId: 9307,
			Content: "新内容", ExpectedRevision: 1,
		})
		require.NoError(t, err)
	})
}
