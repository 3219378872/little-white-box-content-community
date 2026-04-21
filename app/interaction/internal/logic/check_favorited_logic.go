package logic

import (
	"context"
	"errors"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckFavoritedLogic {
	return &CheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CheckFavoritedLogic) CheckFavorited(in *pb.CheckFavoritedReq) (*pb.CheckFavoritedResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return &pb.CheckFavoritedResp{IsFavorited: false}, nil
	}
	if err != nil {
		l.Logger.Errorf("check favorited failed: %v", err)
		return nil, err
	}

	return &pb.CheckFavoritedResp{IsFavorited: record.Status == 1}, nil
}
