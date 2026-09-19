package logic

import (
	"context"
	"errors"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

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
	isFollowing := false
	if in.ViewerId > 0 && in.ViewerId != in.UserId {
		if l.svcCtx.UserFollowModel == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		_, followErr := l.svcCtx.UserFollowModel.FindOneByUserIdTargetUserId(l.ctx, in.ViewerId, in.UserId)
		switch {
		case followErr == nil:
			isFollowing = true
		case errors.Is(followErr, model.ErrNotFound):
		default:
			l.Errorw("UserFollowModel relation lookup failed", logx.Field("err", followErr.Error()))
			return nil, errx.NewWithCode(errx.SystemError)
		}
	}
	return &pb.GetUserResp{
		User:        UserProfileToUserInfo(one),
		IsFollowing: isFollowing,
	}, nil
}
