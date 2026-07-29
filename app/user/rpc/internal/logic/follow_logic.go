package logic

import (
	"context"
	"errors"

	"errx"
	"esx/pkg/event"
	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type FollowLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowLogic {
	return &FollowLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
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
		l.Errorw("build follow behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := l.svcCtx.UserFollowCommands.Follow(l.ctx, in.UserId, in.TargetUserId, outboxEvent); err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.FollowResp{}, nil
}
