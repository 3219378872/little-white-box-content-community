package logic

import (
	"context"

	"errx"
	"esx/app/search/rpc/internal/svc"
	"esx/app/search/rpc/xiaobaihe/search/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetHotSearchesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetHotSearchesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetHotSearchesLogic {
	return &GetHotSearchesLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 获取热门搜索
func (l *GetHotSearchesLogic) GetHotSearches(in *pb.GetHotSearchesReq) (*pb.GetHotSearchesResp, error) {
	if in == nil || in.Limit <= 0 || in.Limit > maxPageSize {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	keywords, err := l.svcCtx.Store.HotSearches(l.ctx, in.Limit)
	if err != nil {
		l.Errorw("aggregate hot searches from tags failed", logx.Field("err", err.Error()))
		return nil, storeError(err)
	}
	return &pb.GetHotSearchesResp{Keywords: keywords}, nil
}
