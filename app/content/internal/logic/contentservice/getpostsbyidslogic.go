package contentservicelogic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPostsByIdsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetPostsByIdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPostsByIdsLogic {
	return &GetPostsByIdsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 按ID批量获取帖子
func (l *GetPostsByIdsLogic) GetPostsByIds(in *pb.GetPostsByIdsReq) (*pb.GetPostsByIdsResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetPostsByIdsResp{}, nil
}
