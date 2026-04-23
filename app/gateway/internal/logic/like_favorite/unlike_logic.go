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

type UnlikeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 取消点赞
func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UnlikeLogic) Unlike(req *types.UnlikeReq) (resp *types.UnlikeResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}

	_, err = l.svcCtx.InteractionService.Unlike(l.ctx, &interactionservice.UnlikeReq{
		UserId:     userId,
		TargetId:   req.TargetId,
		TargetType: req.TargetType,
	})
	if err != nil {
		l.Errorw("InteractionService.Unlike RPC failed",
			logx.Field("userId", userId),
			logx.Field("targetId", req.TargetId),
			logx.Field("targetType", req.TargetType),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &types.UnlikeResp{}, nil
}
