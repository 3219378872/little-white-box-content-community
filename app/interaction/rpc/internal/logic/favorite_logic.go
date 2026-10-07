package logic

import (
	"context"
	"errors"
	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/errx"

	"esx/pkg/logging"
)

// FavoriteLogic 承载 Favorite 接口的业务逻辑；每个请求新建一个实例。
type FavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewFavoriteLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewFavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteLogic {
	return &FavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// Favorite 收藏已发布的帖子：收藏记录、计数与行为事件同事务写入；已收藏时直接成功，不重复计数。
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
	// 事件在事务内按提交后的计数快照构造，下游据此按序号覆盖公开计数。
	recordID, err := l.svcCtx.InteractionCommands.Favorite(l.ctx, in.UserId, in.PostId,
		interactionEventBuilder(in.UserId, in.PostId, "post", "favorite"))
	if err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return &pb.FavoriteResp{}, nil
		}
		l.Errorw("favorite transaction failed",
			logging.Field("userId", in.UserId),
			logging.Field("postId", in.PostId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if l.svcCtx.FavoriteModel != nil {
		if err := l.svcCtx.FavoriteModel.InvalidateFavoriteCache(l.ctx, recordID, in.UserId, in.PostId); err != nil {
			l.Errorw("InvalidateFavoriteCache failed", logging.Field("err", err.Error()))
		}
	}

	return &pb.FavoriteResp{}, nil
}
