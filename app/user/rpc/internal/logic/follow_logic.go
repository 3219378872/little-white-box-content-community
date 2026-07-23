package logic

import (
	"context"
	"errors"

	"errx"
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
	if err := l.svcCtx.UserFollowModel.Follow(l.ctx, in.UserId, in.TargetUserId); err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}

	return &pb.FollowResp{}, nil
}
