package contentservicelogic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreatePostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreatePostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreatePostLogic {
	return &CreatePostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 创建帖子
func (l *CreatePostLogic) CreatePost(in *pb.CreatePostReq) (*pb.CreatePostResp, error) {
	// todo: add your logic here and delete this line

	return &pb.CreatePostResp{}, nil
}
