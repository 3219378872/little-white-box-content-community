package logic

import (
	"context"
	"fmt"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateProfileLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUpdateProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateProfileLogic {
	return &UpdateProfileLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// UpdateProfile 更新用户资料
func (l *UpdateProfileLogic) UpdateProfile(in *pb.UpdateProfileReq) (*pb.UpdateProfileResp, error) {
	err := l.svcCtx.UserProfileModel.UpdateUserDes(l.ctx, in.UserId, in.Nickname, in.AvatarUrl, in.Bio)
	if err != nil {
		return nil, fmt.Errorf("更新用户数据失败, 服务器内部错误:%v", err)
	}
	return &pb.UpdateProfileResp{}, nil
}
