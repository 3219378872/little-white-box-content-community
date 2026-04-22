package contentservicelogic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostsByTagLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostsByTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsByTagLogic {
	return &GetPostsByTagLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取标签下的帖子
func (l *GetPostsByTagLogic) GetPostsByTag(in *pb.GetPostsByTagReq) (*pb.GetPostsByTagResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetPostsByTagResp{}, nil
}
