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

// UnfavoriteLogic 承载 Unfavorite 接口的业务逻辑；每个请求新建一个实例。
type UnfavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewUnfavoriteLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// Unfavorite 取消收藏；不要求帖子仍可见，帖子下线后用户仍能取消。未收藏时直接成功。
func (l *UnfavoriteLogic) Unfavorite(in *pb.UnfavoriteReq) (*pb.UnfavoriteResp, error) {
	if in.UserId <= 0 || in.PostId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return &pb.UnfavoriteResp{}, nil
	}
	if err != nil {
		l.Errorw("FindOneByUserIdPostId failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return &pb.UnfavoriteResp{}, nil
	}

	if l.svcCtx.InteractionCommands == nil {
		l.Errorw("unfavorite command dependency is not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	// 事件在事务内按提交后的计数快照构造，下游据此按序号覆盖公开计数。
	if err = l.svcCtx.InteractionCommands.Unfavorite(l.ctx, record.Id, in.PostId,
		interactionEventBuilder(in.UserId, in.PostId, "post", "unfavorite")); err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return &pb.UnfavoriteResp{}, nil
		}
		l.Errorw("unfavorite transaction failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := l.svcCtx.FavoriteModel.InvalidateFavoriteCache(l.ctx, record.Id, in.UserId, in.PostId); err != nil {
		l.Errorw("InvalidateFavoriteCache failed", logging.Field("err", err.Error()))
	}

	return &pb.UnfavoriteResp{}, nil
}
