package logic

import (
	"context"

	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteMediaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMediaLogic {
	return &DeleteMediaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 删除媒体
func (l *DeleteMediaLogic) DeleteMedia(in *pb.DeleteMediaReq) (*pb.DeleteMediaResp, error) {
	// todo: add your logic here and delete this line

	return &pb.DeleteMediaResp{}, nil
}
