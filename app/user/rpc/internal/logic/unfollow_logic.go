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

type UnfollowLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnfollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfollowLogic {
	return &UnfollowLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 取消关注
func (l *UnfollowLogic) Unfollow(in *pb.UnfollowReq) (*pb.UnfollowResp, error) {
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
	outboxEvent, err := followOutboxEvent(in.UserId, in.TargetUserId, event.BehaviorActionUnfollow)
	if err != nil {
		l.Errorw("build unfollow behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := l.svcCtx.UserFollowCommands.Unfollow(l.ctx, in.UserId, in.TargetUserId, outboxEvent); err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.UnfollowResp{}, nil
}
