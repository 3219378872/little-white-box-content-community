// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package like_favorite

import (
	"context"

	"errx"
	"esx/app/interaction/rpc/interactionservice"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type LikeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 点赞
func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikeLogic) Like(req *types.LikeReq) (resp *types.LikeResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	_, err = l.svcCtx.InteractionService.Like(l.ctx, &interactionservice.LikeReq{
		UserId:     userId,
		TargetId:   req.TargetId,
		TargetType: req.TargetType,
	})
	if err != nil {
		l.Errorw("InteractionService.Like RPC failed",
			logx.Field("userId", userId),
			logx.Field("targetId", req.TargetId),
			logx.Field("targetType", req.TargetType),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.LikeResp{}, nil
}
