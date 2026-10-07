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

// UnlikeLogic 承载 Unlike 接口的业务逻辑；每个请求新建一个实例。
type UnlikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewUnlikeLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// Unlike 取消点赞；不要求目标仍可见，目标下线后用户仍能取消。未点赞时直接成功。
func (l *UnlikeLogic) Unlike(in *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return &pb.UnlikeResp{}, nil
	}
	if err != nil {
		l.Errorw("FindOneByUserIdTargetIdTargetType failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return &pb.UnlikeResp{}, nil
	}

	if l.svcCtx.InteractionCommands == nil {
		l.Errorw("unlike command dependency is not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	// 事件在事务内按提交后的计数快照构造，下游据此按序号覆盖公开计数。
	if err = l.svcCtx.InteractionCommands.Unlike(
		l.ctx, record.Id, in.TargetId, int64(in.TargetType),
		interactionEventBuilder(in.UserId, in.TargetId, targetTypeName(in.TargetType), "unlike"),
	); err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return &pb.UnlikeResp{}, nil
		}
		l.Errorw("unlike transaction failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := l.svcCtx.LikeRecordModel.InvalidateLikeRecordCache(
		l.ctx, record.Id, in.UserId, in.TargetId, int64(in.TargetType),
	); err != nil {
		// CORE-053：权威写入已提交，缓存失效失败只告警。
		l.Errorw("InvalidateLikeRecordCache failed", logging.Field("err", err.Error()))
	}

	return &pb.UnlikeResp{}, nil
}
