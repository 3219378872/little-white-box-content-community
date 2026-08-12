//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserTagsIntegration(t *testing.T) {
	ctx := context.Background()
	_, err := testEnv.DB.ExecContext(ctx,
		"INSERT INTO `user_tag` (`user_id`, `tag_name`, `weight`) VALUES (72001, 'golang', 3), (72001, 'go-zero', 1)")
	require.NoError(t, err)

	resp, err := NewGetUserTagsLogic(ctx, testSvcCtx).GetUserTags(&pb.GetUserTagsReq{UserId: 72001})
	require.NoError(t, err)
	assert.Equal(t, []string{"golang", "go-zero"}, resp.Tags)

	empty, err := NewGetUserTagsLogic(ctx, testSvcCtx).GetUserTags(&pb.GetUserTagsReq{UserId: 72002})
	require.NoError(t, err)
	assert.Empty(t, empty.Tags)
}
