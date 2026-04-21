// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package user

import (
	"context"

	"errx"
	"user/pb/xiaobaihe/user/pb"

	"gateway/internal/svc"
	"gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取用户信息（公开接口）
func NewGetUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserLogic {
	return &GetUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserLogic) GetUser(req *types.GetUserReq) (resp *types.GetUserResp, err error) {
	result, err := l.svcCtx.UserService.GetUser(l.ctx, &pb.GetUserReq{UserId: req.UserId})
	if err != nil {
		l.Errorw("UserService.GetUser RPC failed",
			logx.Field("userId", req.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if result.User == nil {
		return nil, errx.NewWithCode(errx.UserNotFound)
	}
	// DB 1=公开 → true；2=私密 → false
	favoritesVisible := result.User.FavoritesVisibility == 1
	return &types.GetUserResp{
		Id:               result.User.Id,
		Username:         result.User.Username,
		Nickname:         result.User.Nickname,
		AvatarUrl:        result.User.AvatarUrl,
		Bio:              result.User.Bio,
		Level:            result.User.Level,
		FollowerCount:    result.User.FollowerCount,
		FollowingCount:   result.User.FollowingCount,
		PostCount:        result.User.PostCount,
		FavoritesVisible: favoritesVisible,
	}, nil
}
