package logic

import (
	"context"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

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

	result, err := l.svcCtx.LikeRecordModel.UpsertLikeStatus(l.ctx, in.UserId, in.TargetId, int64(in.TargetType), model.StatusActive)
	if err != nil {
		l.Logger.Errorw("UpsertLikeStatus failed",
			logx.Field("userId", in.UserId),
			logx.Field("targetId", in.TargetId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	// RowsAffected: 1 = inserted (new like), 2 = updated (was inactive)
	// Only increment count when the status actually changed to active
	if rowsAffected > 0 {
		if l.svcCtx.ActionCountModel != nil {
			if err := l.svcCtx.ActionCountModel.IncrLikeCount(l.ctx, in.TargetId, int64(in.TargetType)); err != nil {
				l.Logger.Errorw("IncrLikeCount failed",
					logx.Field("targetId", in.TargetId),
					logx.Field("err", err.Error()),
				)
				// Do not fail the request; count inconsistency can be repaired
			}
		}
	} else {
		// Already active, return already liked
		return nil, errx.NewWithCode(errx.AlreadyLiked)
	}

	return &pb.LikeResp{}, nil
}
