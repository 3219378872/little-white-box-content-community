package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckLikedLogic {
	return &CheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 检查是否点赞
func (l *CheckLikedLogic) CheckLiked(in *pb.CheckLikedReq) (*pb.CheckLikedResp, error) {
	// todo: add your logic here and delete this line

	return &pb.CheckLikedResp{}, nil
}
