package contentservicelogic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserPostsLogic {
	return &GetUserPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取用户帖子列表
func (l *GetUserPostsLogic) GetUserPosts(in *pb.GetUserPostsReq) (*pb.GetUserPostsResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetUserPostsResp{}, nil
}
