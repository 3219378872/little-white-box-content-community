package logic

import (
	"context"

	"errx"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFollowersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFollowersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowersLogic {
	return &GetFollowersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取粉丝列表
func (l *GetFollowersLogic) GetFollowers(in *pb.GetFollowersReq) (*pb.GetFollowersResp, error) {
	if in.UserId <= 0 || in.Page <= 0 || in.PageSize <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	offset := int64((in.Page - 1) * in.PageSize)
	limit := int64(in.PageSize)

	users, err := l.svcCtx.UserFollowModel.FindFollowers(l.ctx, in.UserId, offset, limit)
	if err != nil {
		l.Errorw("UserFollowModel.FindFollowers failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	total, err := l.svcCtx.UserFollowModel.CountFollowers(l.ctx, in.UserId)
	if err != nil {
		l.Errorw("UserFollowModel.CountFollowers failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	respUsers := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		respUsers = append(respUsers, UserProfileToUserInfo(user))
	}

	return &pb.GetFollowersResp{Users: respUsers, Total: total}, nil
}
