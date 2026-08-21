// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/jwtx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateProfileLogic 更新用户资料
func NewUpdateProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProfileLogic {
	return &UpdateProfileLogic{
		Logger: logx.WithContext(ctx),
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
		return nil, err
	}
	return &types.UpdateProfileResp{}, nil
}
