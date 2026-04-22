package contentservicelogic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostListLogic {
	return &GetPostListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取帖子列表
func (l *GetPostListLogic) GetPostList(in *pb.GetPostListReq) (*pb.GetPostListResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetPostListResp{}, nil
}
