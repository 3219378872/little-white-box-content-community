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

// UpdateProfileLogic 承载 UpdateProfile 接口的业务逻辑；每个请求新建一个实例。
type UpdateProfileLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateProfileLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUpdateProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProfileLogic {
	return &UpdateProfileLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateProfile 更新当前用户的昵称、头像与简介。
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
