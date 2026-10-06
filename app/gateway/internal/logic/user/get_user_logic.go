package user

import (
	"context"

	pb "esx/kitex_gen/user"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"

	"esx/pkg/logging"
)

// GetUserLogic 承载 GetUser 接口的业务逻辑；每个请求新建一个实例。
type GetUserLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// 获取用户信息（公开接口）
func NewGetUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserLogic {
	return &GetUserLogic{
		Logger: logging.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetUser 返回用户主页资料；带登录态时额外返回是否已关注。
func (l *GetUserLogic) GetUser(req *types.GetUserReq) (resp *types.GetUserResp, err error) {
	viewerID, _ := jwtx.GetOptionalUserIdFromContext(l.ctx)
	result, err := l.svcCtx.UserService.GetUser(l.ctx, &pb.GetUserReq{UserId: req.UserId, ViewerId: viewerID})
	if err != nil {
		l.Errorw("UserService.GetUser RPC failed",
			logging.Field("userId", req.UserId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.FromRPCError(err)
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
		IsFollowing:      result.IsFollowing,
	}, nil
}
