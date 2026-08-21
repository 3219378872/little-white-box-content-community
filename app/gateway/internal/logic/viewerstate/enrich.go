// Package viewerstate 提供当前访问者对帖子列表的互动状态批量查询（CORE-032）。
package viewerstate

import (
	"context"

	"esx/app/gateway/internal/svc"
	"esx/app/interaction/rpc/interactionservice"
	"esx/pkg/errx"
)

// PostTargetType 与 interaction proto 约定一致：1 = post。
const PostTargetType int32 = 1

// Enrich 为当前访问者批量查询 liked/favorited 状态。
// userID <= 0 表示匿名访问者：不查询，返回空映射（公开列表匿名读取）。
// 查询失败返回业务错误；成功但结果缺失按未互动处理。
func Enrich(
	ctx context.Context,
	svcCtx *svc.ServiceContext,
	userID int64,
	postIDs []int64,
) (liked map[int64]bool, favorited map[int64]bool, err error) {
	liked = make(map[int64]bool, len(postIDs))
	favorited = make(map[int64]bool, len(postIDs))
	if userID <= 0 || svcCtx == nil || svcCtx.InteractionService == nil || len(postIDs) == 0 {
		return liked, favorited, nil
	}

	likeResp, err := svcCtx.InteractionService.BatchCheckLiked(ctx, &interactionservice.BatchCheckLikedReq{
		UserId:     userID,
		TargetIds:  postIDs,
		TargetType: PostTargetType,
	})
	if err != nil {
		return nil, nil, errx.FromRPCError(err)
	}
	if likeResp == nil {
		return nil, nil, errx.NewWithCode(errx.SystemError)
	}
	for postID, isLiked := range likeResp.Results {
		liked[postID] = isLiked
	}

	favResp, err := svcCtx.InteractionService.BatchCheckFavorited(ctx, &interactionservice.BatchCheckFavoritedReq{
		UserId:  userID,
		PostIds: postIDs,
	})
	if err != nil {
		return nil, nil, errx.FromRPCError(err)
	}
	if favResp == nil {
		return nil, nil, errx.NewWithCode(errx.SystemError)
	}
	for postID, isFavorited := range favResp.Results {
		favorited[postID] = isFavorited
	}
	return liked, favorited, nil
}
