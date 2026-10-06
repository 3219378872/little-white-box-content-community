package like_favorite

import (
	"context"

	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// UnfavoriteLogic 承载 Unfavorite 接口的业务逻辑；每个请求新建一个实例。
type UnfavoriteLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 取消收藏
func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Unfavorite 取消收藏帖子。
func (l *UnfavoriteLogic) Unfavorite(req *types.UnfavoriteReq) (resp *types.UnfavoriteResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.InteractionService.Unfavorite(l.ctx, &interactionservice.UnfavoriteReq{
		UserId: userId,
		PostId: req.PostId,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "InteractionService.Unfavorite", err,
			logging.Field("userId", userId),
			logging.Field("postId", req.PostId),
		)
	}

	return &types.UnfavoriteResp{}, nil
}
