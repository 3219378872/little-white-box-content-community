package logic

import (
	"context"

	"esx/app/search/rpc/internal/svc"
	pb "esx/kitex_gen/search"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

type GetHotSearchesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewGetHotSearchesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetHotSearchesLogic {
	return &GetHotSearchesLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 获取热门搜索
func (l *GetHotSearchesLogic) GetHotSearches(in *pb.GetHotSearchesReq) (*pb.GetHotSearchesResp, error) {
	if in == nil || in.Limit <= 0 || in.Limit > maxPageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	keywords, err := l.svcCtx.Store.HotSearches(l.ctx, in.Limit)
	if err != nil {
		l.Errorw("aggregate hot searches from tags failed", logging.Field("err", err.Error()))
		return nil, storeError(err)
	}
	return &pb.GetHotSearchesResp{Keywords: keywords}, nil
}
