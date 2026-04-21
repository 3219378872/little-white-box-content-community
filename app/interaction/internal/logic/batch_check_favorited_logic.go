package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchCheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckFavoritedLogic {
	return &BatchCheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *BatchCheckFavoritedLogic) BatchCheckFavorited(in *pb.BatchCheckFavoritedReq) (*pb.BatchCheckFavoritedResp, error) {
	results := make(map[int64]bool, len(in.PostIds))
	for _, postID := range in.PostIds {
		resp, err := NewCheckFavoritedLogic(l.ctx, l.svcCtx).CheckFavorited(&pb.CheckFavoritedReq{
			UserId: in.UserId,
			PostId: postID,
		})
		if err != nil {
			return nil, err
		}
		results[postID] = resp.IsFavorited
	}

	return &pb.BatchCheckFavoritedResp{Results: results}, nil
}
