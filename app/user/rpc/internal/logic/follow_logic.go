package logic

import (
	"context"
	"errors"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	pb "esx/kitex_gen/user"
	"esx/pkg/errx"
	"esx/pkg/event"

	"esx/pkg/logging"
)

// FollowLogic 承载 Follow 接口的业务逻辑；每个请求新建一个实例。
type FollowLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewFollowLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewFollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowLogic {
	return &FollowLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// 关注用户
func (l *FollowLogic) Follow(in *pb.FollowReq) (*pb.FollowResp, error) {
	if in.UserId <= 0 || in.TargetUserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if in.UserId == in.TargetUserId {
		return nil, errx.NewWithCode(errx.CannotFollowSelf)
	}
	if _, err := l.svcCtx.UserProfileModel.FindOne(l.ctx, in.TargetUserId); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.NewWithCode(errx.UserNotFound)
		}
		return nil, errx.Wrap(err, errx.SystemError)
	}
	if l.svcCtx.UserFollowCommands == nil {
		l.Error("UserFollowCommands is not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxEvent, err := followOutboxEvent(in.UserId, in.TargetUserId, event.BehaviorActionFollow)
	if err != nil {
		l.Errorw("build follow behavior event failed", logging.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := l.svcCtx.UserFollowCommands.Follow(l.ctx, in.UserId, in.TargetUserId, outboxEvent); err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.FollowResp{}, nil
}
