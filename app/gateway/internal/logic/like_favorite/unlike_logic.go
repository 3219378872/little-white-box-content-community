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

// UnlikeLogic 承载 Unlike 接口的业务逻辑；每个请求新建一个实例。
type UnlikeLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 取消点赞
func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Unlike 取消点赞帖子或评论。
func (l *UnlikeLogic) Unlike(req *types.UnlikeReq) (resp *types.UnlikeResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.InteractionService.Unlike(l.ctx, &interactionservice.UnlikeReq{
		UserId:     userId,
		TargetId:   req.TargetId,
		TargetType: req.TargetType,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "InteractionService.Unlike", err,
			logging.Field("userId", userId),
			logging.Field("targetId", req.TargetId),
			logging.Field("targetType", req.TargetType),
		)
	}

	return &types.UnlikeResp{}, nil
}
