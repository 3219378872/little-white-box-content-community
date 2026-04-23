package logic

import (
	"context"

	"errx"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/validator"

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
		l.Logger.Errorw("FindFavoriteStatusByUserAndPosts failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	for _, postID := range in.PostIds {
		results[postID] = statusMap[postID]
	}

	return &pb.BatchCheckFavoritedResp{Results: results}, nil
}
