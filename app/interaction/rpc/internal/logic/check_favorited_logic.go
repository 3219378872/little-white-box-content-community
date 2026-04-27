package logic

import (
	"context"
	"errors"
	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"

	"errx"

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
		l.Errorf("check favorited failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.CheckFavoritedResp{IsFavorited: record.Status == model.StatusActive}, nil
}
