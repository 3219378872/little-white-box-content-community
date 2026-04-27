package logic

import (
	"context"

	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserTagsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserTagsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserTagsLogic {
	return &GetUserTagsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取用户标签
func (l *GetUserTagsLogic) GetUserTags(in *pb.GetUserTagsReq) (*pb.GetUserTagsResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetUserTagsResp{}, nil
}
