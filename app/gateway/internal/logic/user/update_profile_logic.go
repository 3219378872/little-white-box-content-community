package user

import (
	"context"

	"esx/app/gateway/internal/logic/rpcx"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	pb "esx/kitex_gen/user"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

type UpdateProfileLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProfileLogic {
	return &UpdateProfileLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateProfileLogic) UpdateProfile(req *types.UpdateProfileReq) (resp *types.UpdateProfileResp, err error) {
	userId, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	_, err = l.svcCtx.UserService.UpdateProfile(l.ctx, &pb.UpdateProfileReq{
		UserId:    userId,
		Nickname:  req.Nickname,
		AvatarUrl: req.AvatarUrl,
		Bio:       req.Bio,
	})
	if err != nil {
		return nil, rpcx.Error(l.Logger, "UserService.UpdateProfile", err)
	}
	return &types.UpdateProfileResp{}, nil
}
