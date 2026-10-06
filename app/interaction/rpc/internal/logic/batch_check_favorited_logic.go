package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/errx"
	"esx/pkg/validator"

	"esx/pkg/logging"
)

type BatchCheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewBatchCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckFavoritedLogic {
	return &BatchCheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

func (l *BatchCheckFavoritedLogic) BatchCheckFavorited(in *pb.BatchCheckFavoritedReq) (*pb.BatchCheckFavoritedResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.PostIds) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	results := make(map[int64]bool, len(in.PostIds))
	if len(in.PostIds) == 0 {
		return &pb.BatchCheckFavoritedResp{Results: results}, nil
	}

	statusMap, err := l.svcCtx.FavoriteModel.FindFavoriteStatusByUserAndPosts(l.ctx, in.UserId, in.PostIds)
	if err != nil {
		l.Errorw("FindFavoriteStatusByUserAndPosts failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	for _, postID := range in.PostIds {
		results[postID] = statusMap[postID]
	}

	return &pb.BatchCheckFavoritedResp{Results: results}, nil
}
