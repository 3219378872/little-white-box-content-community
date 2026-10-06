package logic

import (
	"context"

	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"
	"esx/pkg/errx"
	"esx/pkg/pageutil"

	"esx/pkg/logging"
)

// GetFollowingLogic 承载 GetFollowing 接口的业务逻辑；每个请求新建一个实例。
type GetFollowingLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetFollowingLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetFollowingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFollowingLogic {
	return &GetFollowingLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 获取关注列表
func (l *GetFollowingLogic) GetFollowing(in *pb.GetFollowingReq) (*pb.GetFollowingResp, error) {
	if in.UserId <= 0 || in.Page <= 0 || in.PageSize <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	offset, err := pageutil.PageOffset(in.Page, in.PageSize)
	if err != nil {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	limit := int64(in.PageSize)

	users, err := l.svcCtx.UserFollowModel.FindFollowing(l.ctx, in.UserId, offset, limit)
	if err != nil {
		l.Errorw("UserFollowModel.FindFollowing failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	total, err := l.svcCtx.UserFollowModel.CountFollowing(l.ctx, in.UserId)
	if err != nil {
		l.Errorw("UserFollowModel.CountFollowing failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	respUsers := make([]*pb.UserInfo, 0, len(users))
	for _, user := range users {
		respUsers = append(respUsers, UserProfileToUserInfo(user))
	}

	return &pb.GetFollowingResp{Users: respUsers, Total: total}, nil
}
