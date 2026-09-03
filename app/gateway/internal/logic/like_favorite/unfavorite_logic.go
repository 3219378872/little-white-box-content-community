// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package like_favorite

import (
	"context"

	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/interaction/rpc/interactionservice"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnfavoriteLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 取消收藏
func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UnfavoriteLogic) Unfavorite(req *types.UnfavoriteReq) (resp *types.UnfavoriteResp, err error) {
	userId, err := rpcx.RequireUser(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.InteractionService.Unfavorite(l.ctx, &interactionservice.UnfavoriteReq{
		UserId: userId,
		PostId: req.PostId,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "InteractionService.Unfavorite", err,
			logx.Field("userId", userId),
			logx.Field("postId", req.PostId),
		)
	}

	return &types.UnfavoriteResp{}, nil
}
