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
		l.Errorw("FindOneByUserIdPostId failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}

	if l.svcCtx.InteractionCommands == nil {
		l.Errorw("unfavorite command dependency is not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxEvent, err := interactionOutboxEvent(in.UserId, in.PostId, "post", "unfavorite")
	if err != nil {
		l.Errorw("build unfavorite behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.InteractionCommands.Unfavorite(l.ctx, record.Id, in.PostId, outboxEvent); err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return nil, errx.NewWithCode(errx.NotFavoritedYet)
		}
		l.Errorw("unfavorite transaction failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := invalidateActionCountCache(l.svcCtx, in.PostId, 1); err != nil {
		// CORE-053：权威写入已提交，缓存失效失败只告警。
		l.Errorw("invalidate action count cache failed", logx.Field("err", err.Error()))
	}

	return &pb.UnfavoriteResp{}, nil
}
