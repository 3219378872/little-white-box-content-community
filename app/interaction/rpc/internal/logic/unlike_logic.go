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

type UnlikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnlikeLogic) Unlike(in *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}
	if err != nil {
		l.Errorw("FindOneByUserIdTargetIdTargetType failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}

	if l.svcCtx.InteractionCommands == nil {
		l.Errorw("unlike command dependency is not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxEvent, err := interactionOutboxEvent(
		in.UserId, in.TargetId, targetTypeName(in.TargetType), "unlike",
	)
	if err != nil {
		l.Errorw("build unlike behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err = l.svcCtx.InteractionCommands.Unlike(
		l.ctx, record.Id, in.TargetId, int64(in.TargetType), outboxEvent,
	); err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return nil, errx.NewWithCode(errx.NotLikedYet)
		}
		l.Errorw("unlike transaction failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := l.svcCtx.LikeRecordModel.InvalidateLikeRecordCache(
		l.ctx, record.Id, in.UserId, in.TargetId, int64(in.TargetType),
	); err != nil {
		l.Errorw("InvalidateLikeRecordCache failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := invalidateActionCountCache(l.svcCtx, in.TargetId, int64(in.TargetType)); err != nil {
		l.Errorw("invalidate action count cache failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.UnlikeResp{}, nil
}
