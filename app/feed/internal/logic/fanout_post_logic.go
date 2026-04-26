package logic

import (
	"context"

	"esx/app/feed/internal/svc"
	"esx/app/feed/xiaobaihe/feed/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type FanoutPostLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFanoutPostLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FanoutPostLogic {
	return &FanoutPostLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FanoutPostLogic) FanoutPost(in *pb.FanoutPostReq) (*pb.FanoutPostResp, error) {
	// todo: add your logic here and delete this line

	return &pb.FanoutPostResp{}, nil
}
