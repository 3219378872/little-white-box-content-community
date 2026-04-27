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

	result, err := l.svcCtx.LikeRecordModel.UpdateStatusById(l.ctx, record.Id, model.StatusActive, model.StatusInactive)
	if err != nil {
		l.Errorw("UpdateStatusById failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		if l.svcCtx.ActionCountModel != nil {
			if err := l.svcCtx.ActionCountModel.DecrLikeCount(l.ctx, in.TargetId, int64(in.TargetType)); err != nil {
				l.Errorw("DecrLikeCount failed", logx.Field("err", err.Error()))
			}
		}
	}

	return &pb.UnlikeResp{}, nil
}
