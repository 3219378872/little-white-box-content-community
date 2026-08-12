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

type LikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if l.svcCtx.InteractionCommands == nil || l.svcCtx.LikeRecordModel == nil || l.svcCtx.ActionCountModel == nil {
		l.Errorw("like dependencies are not configured")
		return nil, errx.NewWithCode(errx.SystemError)
	}
	outboxEvent, err := interactionOutboxEvent(
		in.UserId, in.TargetId, targetTypeName(in.TargetType), "like",
	)
	if err != nil {
		l.Errorw("build like behavior event failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	likeRecordID, err := l.svcCtx.InteractionCommands.Like(
		l.ctx, in.UserId, in.TargetId, int64(in.TargetType), outboxEvent,
	)
	if err != nil {
		if errors.Is(err, model.ErrNoStateChange) {
			return nil, errx.NewWithCode(errx.AlreadyLiked)
		}
		l.Errorw("local like transaction failed",
			logx.Field("userId", in.UserId),
			logx.Field("targetId", in.TargetId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.svcCtx.LikeRecordModel.InvalidateLikeRecordCache(l.ctx, likeRecordID, in.UserId, in.TargetId, int64(in.TargetType)); err != nil {
		// CORE-053：权威写入已提交，缓存失效失败不得把响应改成可重试失败。
		l.Errorw("InvalidateLikeRecordCache failed", logx.Field("err", err.Error()))
	}
	if err := invalidateActionCountCache(l.svcCtx, in.TargetId, int64(in.TargetType)); err != nil {
		l.Errorw("invalidate action count cache failed", logx.Field("err", err.Error()))
	}

	return &pb.LikeResp{}, nil
}
