package logic

import (
	"context"
	"errors"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnfavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnfavoriteLogic) Unfavorite(in *pb.UnfavoriteReq) (*pb.UnfavoriteResp, error) {
	if in.UserId <= 0 || in.PostId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}
	if err != nil {
		l.Logger.Errorw("FindOneByUserIdPostId failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}

	result, err := l.svcCtx.FavoriteModel.UpdateStatusById(l.ctx, record.Id, model.StatusActive, model.StatusInactive)
	if err != nil {
		l.Logger.Errorw("UpdateStatusById failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		if l.svcCtx.ActionCountModel != nil {
			if err := l.svcCtx.ActionCountModel.DecrFavoriteCount(l.ctx, in.PostId, 1); err != nil {
				l.Logger.Errorw("DecrFavoriteCount failed", logx.Field("err", err.Error()))
			}
		}
	}

	return &pb.UnfavoriteResp{}, nil
}
