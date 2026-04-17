package logic

import (
	"context"

	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchGetMediaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchGetMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchGetMediaLogic {
	return &BatchGetMediaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 批量获取媒体信息
func (l *BatchGetMediaLogic) BatchGetMedia(in *pb.BatchGetMediaReq) (*pb.BatchGetMediaResp, error) {
	// todo: add your logic here and delete this line

	return &pb.BatchGetMediaResp{}, nil
}
