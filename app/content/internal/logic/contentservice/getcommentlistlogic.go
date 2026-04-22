package contentservicelogic

import (
	"context"

	"esx/app/content/internal/svc"
	"esx/app/content/pb/xiaobaihe/content/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCommentListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCommentListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCommentListLogic {
	return &GetCommentListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取评论列表
func (l *GetCommentListLogic) GetCommentList(in *pb.GetCommentListReq) (*pb.GetCommentListResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetCommentListResp{}, nil
}
