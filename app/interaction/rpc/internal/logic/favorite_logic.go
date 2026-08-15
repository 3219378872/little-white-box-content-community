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
	if err := requirePublishedPost(l.ctx, l.svcCtx.ContentService, in.PostId); err != nil {
		return nil, err
	}

	if l.svcCtx.InteractionCommands == nil {
		l.Errorw("favorite command dependency is not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxEvent, err := interactionOutboxEvent(in.UserId, in.PostId, "post", "favorite")
	if err != nil {
		l.Errorw("build favorite behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	_, err = l.svcCtx.InteractionCommands.Favorite(l.ctx, in.UserId, in.PostId, outboxEvent)
	if err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return &pb.FavoriteResp{}, nil
		}
		l.Errorw("favorite transaction failed",
			logx.Field("userId", in.UserId),
			logx.Field("postId", in.PostId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := invalidateActionCountCache(l.svcCtx, in.PostId, 1); err != nil {
		// CORE-053：权威写入已提交，缓存失效失败只告警。
		l.Errorw("invalidate action count cache failed", logx.Field("err", err.Error()))
	}

	return &pb.FavoriteResp{}, nil
}
