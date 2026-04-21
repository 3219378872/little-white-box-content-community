package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFavoriteListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFavoriteListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFavoriteListLogic {
	return &GetFavoriteListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetFavoriteListLogic) GetFavoriteList(in *pb.GetFavoriteListReq) (*pb.GetFavoriteListResp, error) {
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 100 {
		in.PageSize = 20
	}

	postIDs, total, err := l.svcCtx.FavoriteModel.FindActivePostIds(l.ctx, in.UserId, in.Page, in.PageSize)
	if err != nil {
		l.Logger.Errorf("get favorite list failed: %v", err)
		return nil, err
	}

	return &pb.GetFavoriteListResp{
		PostIds: postIDs,
		Total:   total,
	}, nil
}
