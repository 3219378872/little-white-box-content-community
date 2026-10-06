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

type LikeLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 点赞
func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LikeLogic) Like(req *types.LikeReq) (resp *types.LikeResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.InteractionService.Like(l.ctx, &interactionservice.LikeReq{
		UserId:     userId,
		TargetId:   req.TargetId,
		TargetType: req.TargetType,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "InteractionService.Like", err,
			logging.Field("userId", userId),
			logging.Field("targetId", req.TargetId),
			logging.Field("targetType", req.TargetType),
		)
	}

	return &types.LikeResp{}, nil
}
