//go:build integration

package logic

import (
	"context"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLikeUnlikeIntegration(t *testing.T) {
	resetIntegrationState()

	ctx := context.Background()
	likeLogic := NewLikeLogic(ctx, testSvcCtx)
	unlikeLogic := NewUnlikeLogic(ctx, testSvcCtx)
	checkLogic := NewCheckLikedLogic(ctx, testSvcCtx)
	countsLogic := NewGetCountsLogic(ctx, testSvcCtx)

	_, err := likeLogic.Like(&pb.LikeReq{
		UserId:     7001,
		TargetId:   900001,
		TargetType: 1,
	})
	require.NoError(t, err)

	checkResp, err := checkLogic.CheckLiked(&pb.CheckLikedReq{
		UserId:     7001,
		TargetId:   900001,
		TargetType: 1,
	})
	require.NoError(t, err)
	require.True(t, checkResp.IsLiked)

	countsResp, err := countsLogic.GetCounts(&pb.GetCountsReq{
		TargetId:   900001,
		TargetType: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), countsResp.LikeCount)

	var status int64
	err = testDB.QueryRow("SELECT `status` FROM `like_record` WHERE `user_id`=? AND `target_id`=? AND `target_type`=?",
		7001, 900001, 1).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, int64(1), status)

	_, err = unlikeLogic.Unlike(&pb.UnlikeReq{
		UserId:     7001,
		TargetId:   900001,
		TargetType: 1,
	})
	require.NoError(t, err)

	checkResp, err = checkLogic.CheckLiked(&pb.CheckLikedReq{
		UserId:     7001,
		TargetId:   900001,
		TargetType: 1,
	})
	require.NoError(t, err)
	require.False(t, checkResp.IsLiked)

	countsResp, err = countsLogic.GetCounts(&pb.GetCountsReq{
		TargetId:   900001,
		TargetType: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), countsResp.LikeCount)

	err = testDB.QueryRow("SELECT `status` FROM `like_record` WHERE `user_id`=? AND `target_id`=? AND `target_type`=?",
		7001, 900001, 1).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, int64(0), status)
}

func TestFavoriteListIntegration(t *testing.T) {
	resetIntegrationState()

	_, err := testDB.Exec(`
		INSERT INTO favorite (user_id, post_id, status, created_at, updated_at)
		VALUES
			(8001, 910101, 1, '2026-04-21 10:00:00', '2026-04-21 10:00:00'),
			(8001, 910102, 1, '2026-04-21 11:00:00', '2026-04-21 11:00:00'),
			(8001, 910103, 0, '2026-04-21 12:00:00', '2026-04-21 12:00:00')
	`)
	require.NoError(t, err)

	ctx := context.Background()
	listLogic := NewGetFavoriteListLogic(ctx, testSvcCtx)
	checkLogic := NewCheckFavoritedLogic(ctx, testSvcCtx)
	batchLogic := NewBatchCheckFavoritedLogic(ctx, testSvcCtx)

	listResp, err := listLogic.GetFavoriteList(&pb.GetFavoriteListReq{
		UserId:   8001,
		Page:     1,
		PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []int64{910102, 910101}, listResp.PostIds)
	require.Equal(t, int64(2), listResp.Total)

	checkResp, err := checkLogic.CheckFavorited(&pb.CheckFavoritedReq{
		UserId: 8001,
		PostId: 910101,
	})
	require.NoError(t, err)
	require.True(t, checkResp.IsFavorited)

	checkResp, err = checkLogic.CheckFavorited(&pb.CheckFavoritedReq{
		UserId: 8001,
		PostId: 910103,
	})
	require.NoError(t, err)
	require.False(t, checkResp.IsFavorited)

	batchResp, err := batchLogic.BatchCheckFavorited(&pb.BatchCheckFavoritedReq{
		UserId:  8001,
		PostIds: []int64{910101, 910103, 910104},
	})
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{
		910101: true,
		910103: false,
		910104: false,
	}, batchResp.Results)
}

func TestGetCountsCacheBackfillIntegration(t *testing.T) {
	resetIntegrationState()

	_, err := testDB.Exec(`
		INSERT INTO action_count (target_id, target_type, like_count, favorite_count, comment_count, share_count)
		VALUES (?, ?, ?, ?, ?, ?)
	`, 920001, 1, 7, 3, 2, 0)
	require.NoError(t, err)

	ctx := context.Background()
	countsLogic := NewGetCountsLogic(ctx, testSvcCtx)
	likeCountLogic := NewGetLikeCountLogic(ctx, testSvcCtx)

	countsResp, err := countsLogic.GetCounts(&pb.GetCountsReq{
		TargetId:   920001,
		TargetType: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), countsResp.LikeCount)
	require.Equal(t, int64(3), countsResp.FavoriteCount)
	require.Equal(t, int64(2), countsResp.CommentCount)

	cachedLike, err := testSvcCtx.Redis.Hget("interaction:action_count:920001:1", "like_count")
	require.NoError(t, err)
	require.Equal(t, "7", cachedLike)

	cachedFavorite, err := testSvcCtx.Redis.Hget("interaction:action_count:920001:1", "favorite_count")
	require.NoError(t, err)
	require.Equal(t, "3", cachedFavorite)

	likeResp, err := likeCountLogic.GetLikeCount(&pb.GetLikeCountReq{
		TargetId:   920001,
		TargetType: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), likeResp.Count)
}
