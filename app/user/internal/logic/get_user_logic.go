package logic

import (
	"context"
	"errors"

	"errx"
	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserLogic {
	return &GetUserLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取用户信息
func (l *GetUserLogic) GetUser(in *pb.GetUserReq) (*pb.GetUserResp, error) {
	one, err := l.svcCtx.UserProfileModel.FindOne(l.ctx, in.UserId)
	if err != nil {
		l.Errorw("UserProfileModel.FindOne failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.UserNotFound)
		}
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.GetUserResp{
		User: UserProfileToUserInfo(one),
	}, nil
}
