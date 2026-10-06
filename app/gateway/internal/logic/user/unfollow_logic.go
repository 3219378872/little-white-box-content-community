package user

import (
	"context"

	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// UnfollowLogic 承载 Unfollow 接口的业务逻辑；每个请求新建一个实例。
type UnfollowLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 取消关注
func NewUnfollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfollowLogic {
	return &UnfollowLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Unfollow 取消当前用户对目标用户的关注。
func (l *UnfollowLogic) Unfollow(req *types.UnfollowReq) (resp *types.UnfollowResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if _, err = l.svcCtx.UserService.Unfollow(l.ctx, &userservice.UnfollowReq{
		UserId:       userID,
		TargetUserId: req.TargetUserId,
	}); err != nil {
		return nil, rpcx.Error(l.Logger, "UserService.Unfollow", err)
	}

	return &types.UnfollowResp{}, nil
}
