package logic

import (
	"context"

	"errx"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

const maxBatchGetUsers = 100

type BatchGetUsersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchGetUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchGetUsersLogic {
	return &BatchGetUsersLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 批量获取用户信息
func (l *BatchGetUsersLogic) BatchGetUsers(in *pb.BatchGetUsersReq) (*pb.BatchGetUsersResp, error) {
	if in == nil || len(in.UserIds) == 0 || len(in.UserIds) > maxBatchGetUsers {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	ids := make([]int64, 0, len(in.UserIds))
	seen := make(map[int64]struct{}, len(in.UserIds))
	for _, id := range in.UserIds {
		if id <= 0 {
			return nil, errx.NewWithCode(errx.ParamError)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	profiles, err := l.svcCtx.UserProfileModel.FindByIDs(l.ctx, ids)
	if err != nil {
		l.Errorw("UserProfileModel.FindByIDs failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	byID := make(map[int64]*pb.UserInfo, len(profiles))
	for _, profile := range profiles {
		if profile != nil {
			byID[profile.Id] = UserProfileToUserInfo(profile)
		}
	}
	users := make([]*pb.UserInfo, 0, len(byID))
	for _, id := range ids {
		if user := byID[id]; user != nil {
			users = append(users, user)
		}
	}

	return &pb.BatchGetUsersResp{Users: users}, nil
}
