package logic

import (
	"context"

	"esx/app/media/internal/svc"
	"esx/app/media/pb/xiaobaihe/media/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaLogic {
	return &GetMediaLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取媒体信息
func (l *GetMediaLogic) GetMedia(in *pb.GetMediaReq) (*pb.GetMediaResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetMediaResp{}, nil
}
