package logic

import (
	"context"

	"errx"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFollowingLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFollowingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowingLogic {
	return &GetFollowingLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取关注列表
func (l *GetFollowingLogic) GetFollowing(in *pb.GetFollowingReq) (*pb.GetFollowingResp, error) {
	if in.UserId <= 0 || in.Page <= 0 || in.PageSize <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	offset := int64((in.Page - 1) * in.PageSize)
	limit := int64(in.PageSize)

	users, err := l.svcCtx.UserFollowModel.FindFollowing(l.ctx, in.UserId, offset, limit)
	if err != nil {
		l.Errorw("UserFollowModel.FindFollowing failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	total, err := l.svcCtx.UserFollowModel.CountFollowing(l.ctx, in.UserId)
	if err != nil {
		l.Errorw("UserFollowModel.CountFollowing failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	respUsers := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		respUsers = append(respUsers, UserProfileToUserInfo(user))
	}

	return &pb.GetFollowingResp{Users: respUsers, Total: total}, nil
}
