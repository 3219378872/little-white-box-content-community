package logic

import (
	"context"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type FavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteLogic {
	return &FavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FavoriteLogic) Favorite(in *pb.FavoriteReq) (*pb.FavoriteResp, error) {
	if in.UserId <= 0 || in.PostId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	result, err := l.svcCtx.FavoriteModel.UpsertFavoriteStatus(l.ctx, in.UserId, in.PostId, model.StatusActive)
	if err != nil {
		l.Logger.Errorw("UpsertFavoriteStatus failed",
			logx.Field("userId", in.UserId),
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		if l.svcCtx.ActionCountModel != nil {
			if err := l.svcCtx.ActionCountModel.IncrFavoriteCount(l.ctx, in.PostId, 1); err != nil {
				l.Logger.Errorw("IncrFavoriteCount failed",
					logx.Field("postId", in.PostId),
					logx.Field("err", err.Error()),
				)
			}
		}
	} else {
		return nil, errx.NewWithCode(errx.AlreadyFavorited)
	}

	return &pb.FavoriteResp{}, nil
}
