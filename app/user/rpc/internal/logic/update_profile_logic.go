package logic

import (
	"context"

	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"
	"esx/pkg/errx"

	logx "esx/pkg/logging"
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
		l.Errorw("UserProfileModel.UpdateUserDes failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.UpdateProfileResp{}, nil
}
