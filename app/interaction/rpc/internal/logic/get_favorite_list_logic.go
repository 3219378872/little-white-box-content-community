package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"

	"esx/pkg/errx"
	"esx/pkg/pageutil"

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
	page := pageutil.ClampPage(in.Page)
	pageSize := pageutil.ClampPageSizeTo(in.PageSize, pageutil.DefaultPageSize, pageutil.InteractionMaxPageSize)

	postIDs, total, err := l.svcCtx.FavoriteModel.FindActivePostIds(l.ctx, in.UserId, page, pageSize)
	if err != nil {
		l.Errorf("get favorite list failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.GetFavoriteListResp{
		PostIds: postIDs,
		Total:   total,
	}, nil
}
