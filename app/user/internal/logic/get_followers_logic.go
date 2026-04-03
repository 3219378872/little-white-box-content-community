package logic

import (
	"context"

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
	// todo: add your logic here and delete this line

	return &pb.GetFollowersResp{}, nil
}
