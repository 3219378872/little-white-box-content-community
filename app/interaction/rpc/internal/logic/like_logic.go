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

// LikeLogic 承载 Like 接口的业务逻辑；每个请求新建一个实例。
type LikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewLikeLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// Like 点赞已发布的帖子或评论：点赞记录、计数与行为事件同事务写入；已点赞时直接成功，不重复计数。
func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if err := requirePublishedLikeTarget(l.ctx, l.svcCtx.ContentService, in.TargetId, in.TargetType); err != nil {
		return nil, err
	}
	if l.svcCtx.InteractionCommands == nil || l.svcCtx.LikeRecordModel == nil || l.svcCtx.ActionCountModel == nil {
		l.Errorw("like dependencies are not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxEvent, err := interactionOutboxEvent(
		in.UserId, in.TargetId, targetTypeName(in.TargetType), "like",
	)
	if err != nil {
		l.Errorw("build like behavior event failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	likeRecordID, err := l.svcCtx.InteractionCommands.Like(
		l.ctx, in.UserId, in.TargetId, int64(in.TargetType), outboxEvent,
	)
	if err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return &pb.LikeResp{}, nil
		}
		l.Errorw("local like transaction failed",
			logging.Field("userId", in.UserId),
			logging.Field("targetId", in.TargetId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.svcCtx.LikeRecordModel.InvalidateLikeRecordCache(l.ctx, likeRecordID, in.UserId, in.TargetId, int64(in.TargetType)); err != nil {
		// CORE-053：权威写入已提交，缓存失效失败不得把响应改成可重试失败。
		l.Errorw("InvalidateLikeRecordCache failed", logging.Field("err", err.Error()))
	}
	if err := invalidateActionCountCache(l.ctx, l.svcCtx, in.TargetId, int64(in.TargetType)); err != nil {
		l.Errorw("invalidate action count cache failed", logging.Field("err", err.Error()))
	}

	return &pb.LikeResp{}, nil
}
