// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package like_favorite

import (
	"context"

	"esx/app/interaction/interactionservice"
	"errx"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

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
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	_, err = l.svcCtx.InteractionService.Unfavorite(l.ctx, &interactionservice.UnfavoriteReq{
		UserId: userId,
		PostId: req.PostId,
	})
	if err != nil {
		l.Errorw("InteractionService.Unfavorite RPC failed",
			logx.Field("userId", userId),
			logx.Field("postId", req.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.UnfavoriteResp{}, nil
}
