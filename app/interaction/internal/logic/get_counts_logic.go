package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCountsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCountsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCountsLogic {
	return &GetCountsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取互动统计
func (l *GetCountsLogic) GetCounts(in *pb.GetCountsReq) (*pb.GetCountsResp, error) {
	// todo: add your logic here and delete this line

	return &pb.GetCountsResp{}, nil
}
