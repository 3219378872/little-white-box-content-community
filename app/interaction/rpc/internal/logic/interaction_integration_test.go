//go:build integration

package logic

import (
	"context"
	"encoding/json"
	pb "esx/kitex_gen/interaction"
	"testing"

	"esx/pkg/event"

	"github.com/stretchr/testify/require"
)

// actionCountRow 读取 action_count 中目标的点赞数与序号。
func actionCountRow(t *testing.T, targetID, targetType int64) (likeCount, seq int64) {
	t.Helper()
	require.NoError(t, testEnv.DB.QueryRow(
		"SELECT `like_count`, `count_seq` FROM `action_count` WHERE `target_id`=? AND `target_type`=?",
		targetID, targetType).Scan(&likeCount, &seq))
	return likeCount, seq
}

// latestBehaviorSnapshot 解码最近一条行为 outbox 事件携带的计数快照。
func latestBehaviorSnapshot(t *testing.T) event.InteractionCountSnapshot {
	t.Helper()
	var payload []byte
	require.NoError(t, testEnv.DB.QueryRow(
		"SELECT `payload` FROM `event_outbox` WHERE `topic`='user-behavior-v2' ORDER BY `id` DESC LIMIT 1").Scan(&payload))
	var behavior event.BehaviorEvent
	require.NoError(t, json.Unmarshal(payload, &behavior))
	require.NotNil(t, behavior.CountSnapshot, "interaction events must carry the committed count snapshot")
	return *behavior.CountSnapshot
}

func TestLikeUnlikeIntegration(t *testing.T) {
	resetIntegrationState()

	ctx := context.Background()
	likeLogic := NewLikeLogic(ctx, testSvcCtx)
	unlikeLogic := NewUnlikeLogic(ctx, testSvcCtx)
	checkLogic := NewCheckLikedLogic(ctx, testSvcCtx)

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

	// 计数与序号在同一事务提交，事件携带的快照与表中一致。
	likeCount, seq := actionCountRow(t, 900001, 1)
	require.Equal(t, int64(1), likeCount)
	require.Equal(t, int64(1), seq)
	require.Equal(t, event.InteractionCountSnapshot{LikeCount: 1, Seq: 1}, latestBehaviorSnapshot(t))

	// 重复点赞无状态变化：计数与序号都不推进。
	_, err = likeLogic.Like(&pb.LikeReq{UserId: 7001, TargetId: 900001, TargetType: 1})
	require.NoError(t, err)
	_, seq = actionCountRow(t, 900001, 1)
	require.Equal(t, int64(1), seq)

	var status int64
	err = testEnv.DB.QueryRow("SELECT `status` FROM `like_record` WHERE `user_id`=? AND `target_id`=? AND `target_type`=?",
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

	// 取消点赞同样推进序号，下游据此用更新的快照覆盖旧计数。
	likeCount, seq = actionCountRow(t, 900001, 1)
	require.Equal(t, int64(0), likeCount)
	require.Equal(t, int64(2), seq)
	require.Equal(t, event.InteractionCountSnapshot{LikeCount: 0, Seq: 2}, latestBehaviorSnapshot(t))

	err = testEnv.DB.QueryRow("SELECT `status` FROM `like_record` WHERE `user_id`=? AND `target_id`=? AND `target_type`=?",
		7001, 900001, 1).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, int64(0), status)
}

func TestFavoriteListIntegration(t *testing.T) {
	resetIntegrationState()

	_, err := testEnv.DB.Exec(`
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
