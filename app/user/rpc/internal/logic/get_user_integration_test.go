//go:build integration

package logic

import (
	"context"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestGetUserReadsCurrentViewerRelationIntegration(t *testing.T) {
	// TestMain supplies disposable MySQL/Redis containers, never the dev database.
	testEnv.TruncateAll(t, "user_follow", "user_profile")
	ctx := context.Background()
	for _, id := range []int64{1, 2, 3} {
		_, err := testSvcCtx.UserProfileModel.Insert(ctx, sampleUser(id, string(rune('a'+id))))
		require.NoError(t, err)
	}
	follows := model.NewUserFollowModel(testSvcCtx.DB)
	read := func(viewer int64, want bool) {
		t.Helper()
		response, err := NewGetUserLogic(ctx, testSvcCtx).GetUser(&pb.GetUserReq{UserId: 2, ViewerId: viewer})
		require.NoError(t, err)
		require.Equal(t, want, response.IsFollowing)
	}
	read(1, false)
	require.NoError(t, follows.Follow(ctx, 1, 2))
	read(1, true)
	read(0, false)
	read(2, false)
	read(3, false)
	require.NoError(t, follows.Unfollow(ctx, 1, 2))
	read(1, false)
}
