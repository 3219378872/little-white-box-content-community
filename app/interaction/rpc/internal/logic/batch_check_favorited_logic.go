package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/errx"
	"esx/pkg/validator"

	"esx/pkg/logging"
)

// BatchCheckFavoritedLogic 承载 BatchCheckFavorited 接口的业务逻辑；每个请求新建一个实例。
type BatchCheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewBatchCheckFavoritedLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewBatchCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckFavoritedLogic {
	return &BatchCheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// BatchCheckFavorited 批量查询用户是否收藏了各帖子；未收藏或从未收藏的帖子返回 false。
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
