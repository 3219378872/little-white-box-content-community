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
