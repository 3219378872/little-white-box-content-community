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

// FollowLogic 承载 Follow 接口的业务逻辑；每个请求新建一个实例。
type FollowLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 关注用户
func NewFollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowLogic {
	return &FollowLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Follow 让当前用户关注目标用户。
func (l *FollowLogic) Follow(req *types.FollowReq) (resp *types.FollowResp, err error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if _, err = l.svcCtx.UserService.Follow(l.ctx, &userservice.FollowReq{
		UserId:       userID,
		TargetUserId: req.TargetUserId,
	}); err != nil {
		return nil, rpcx.Error(l.Logger, "UserService.Follow", err)
	}

	return &types.FollowResp{}, nil
}
