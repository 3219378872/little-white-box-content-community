package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/logging"
)

type GetLikeCountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetLikeCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikeCountLogic {
	return &GetLikeCountLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
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
