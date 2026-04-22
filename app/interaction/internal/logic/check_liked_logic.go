package logic

import (
	"context"
	"errors"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckLikedLogic {
	return &CheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CheckLikedLogic) CheckLiked(in *pb.CheckLikedReq) (*pb.CheckLikedResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return &pb.CheckLikedResp{IsLiked: false}, nil
	}
	if err != nil {
		l.Logger.Errorf("check liked failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.CheckLikedResp{IsLiked: record.Status == model.StatusActive}, nil
}
