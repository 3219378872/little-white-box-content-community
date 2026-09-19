//go:build integration

package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"

	"github.com/stretchr/testify/require"
)

func TestCancellationIgnoresStaleRelationCacheIntegration(t *testing.T) {
	ctx := context.Background()
	for i, stale := range []string{"negative", "inactive"} {
		t.Run(stale, func(t *testing.T) {
			userID, postID := int64(45000+i), int64(95000+i)
			var outboxBefore int64
			require.NoError(t, testEnv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_outbox").Scan(&outboxBefore))
			_, err := NewLikeLogic(ctx, testSvcCtx).Like(&pb.LikeReq{UserId: userID, TargetId: postID, TargetType: 1})
			require.NoError(t, err)
			_, err = NewFavoriteLogic(ctx, testSvcCtx).Favorite(&pb.FavoriteReq{UserId: userID, PostId: postID})
			require.NoError(t, err)
			like, err := testSvcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(ctx, userID, postID, 1)
			require.NoError(t, err)
			favorite, err := testSvcCtx.FavoriteModel.FindOneByUserIdPostId(ctx, userID, postID)
			require.NoError(t, err)
			likeIndex := fmt.Sprintf("cache:likeRecord:userId:targetId:targetType:%d:%d:1", userID, postID)
			favoriteIndex := fmt.Sprintf("cache:favorite:userId:postId:%d:%d", userID, postID)
			// Simulate the interval between a committed activation and a delayed
			// cache deletion retry, including both index misses and stale records.
			if stale == "negative" {
				require.NoError(t, testEnv.Redis.SetexCtx(ctx, likeIndex, "*", 120))
				require.NoError(t, testEnv.Redis.SetexCtx(ctx, favoriteIndex, "*", 120))
			} else {
				like.Status, favorite.Status = model.StatusInactive, model.StatusInactive
				likeJSON, err := json.Marshal(like)
				require.NoError(t, err)
				favoriteJSON, err := json.Marshal(favorite)
				require.NoError(t, err)
				require.NoError(t, testEnv.Redis.SetexCtx(ctx, likeIndex, strconv.FormatInt(like.Id, 10), 120))
				require.NoError(t, testEnv.Redis.SetexCtx(ctx, favoriteIndex, strconv.FormatInt(favorite.Id, 10), 120))
				require.NoError(t, testEnv.Redis.SetexCtx(ctx, fmt.Sprintf("cache:likeRecord:id:%d", like.Id), string(likeJSON), 120))
				require.NoError(t, testEnv.Redis.SetexCtx(ctx, fmt.Sprintf("cache:favorite:id:%d", favorite.Id), string(favoriteJSON), 120))
			}
			// Repeating cancellation must not decrement counts or enqueue twice.
			for range 2 {
				_, err = NewUnlikeLogic(ctx, testSvcCtx).Unlike(&pb.UnlikeReq{UserId: userID, TargetId: postID, TargetType: 1})
				require.NoError(t, err)
				_, err = NewUnfavoriteLogic(ctx, testSvcCtx).Unfavorite(&pb.UnfavoriteReq{UserId: userID, PostId: postID})
				require.NoError(t, err)
			}
			var status, count int64
			require.NoError(t, testEnv.DB.QueryRowContext(ctx, "SELECT status FROM like_record WHERE id=?", like.Id).Scan(&status))
			require.Equal(t, int64(model.StatusInactive), status)
			require.NoError(t, testEnv.DB.QueryRowContext(ctx, "SELECT status FROM favorite WHERE id=?", favorite.Id).Scan(&status))
			require.Equal(t, int64(model.StatusInactive), status)
			require.NoError(t, testEnv.DB.QueryRowContext(ctx, "SELECT like_count + favorite_count FROM action_count WHERE target_id=? AND target_type=1", postID).Scan(&count))
			require.Zero(t, count)
			require.NoError(t, testEnv.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_outbox").Scan(&count))
			require.Equal(t, int64(4), count-outboxBefore, "like/favorite and their cancellations each enqueue once")
		})
	}
}
