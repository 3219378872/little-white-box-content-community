package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLikeCountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLikeCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikeCountLogic {
	return &GetLikeCountLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLikeCountLogic) GetLikeCount(in *pb.GetLikeCountReq) (*pb.GetLikeCountResp, error) {
	resp, err := NewGetCountsLogic(l.ctx, l.svcCtx).GetCounts(&pb.GetCountsReq{
		TargetId:   in.TargetId,
		TargetType: in.TargetType,
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetLikeCountResp{Count: resp.LikeCount}, nil
}
