package logic

import (
	"context"

	"errx"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchCheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckLikedLogic {
	return &BatchCheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *BatchCheckLikedLogic) BatchCheckLiked(in *pb.BatchCheckLikedReq) (*pb.BatchCheckLikedResp, error) {
	results := make(map[int64]bool, len(in.TargetIds))
	for _, targetID := range in.TargetIds {
		resp, err := NewCheckLikedLogic(l.ctx, l.svcCtx).CheckLiked(&pb.CheckLikedReq{
			UserId:     in.UserId,
			TargetId:   targetID,
			TargetType: in.TargetType,
		})
		if err != nil {
			l.Logger.Errorf("batch check liked failed for target %d: %v", targetID, err)
			return nil, errx.NewWithCode(errx.SystemError)
		}
		results[targetID] = resp.IsLiked
	}

	return &pb.BatchCheckLikedResp{Results: results}, nil
}
