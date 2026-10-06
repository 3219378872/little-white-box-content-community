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

type CheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckLikedLogic {
	return &CheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

func (l *CheckLikedLogic) CheckLiked(in *pb.CheckLikedReq) (*pb.CheckLikedResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return &pb.CheckLikedResp{IsLiked: false}, nil
	}
	if err != nil {
		l.Errorf("check liked failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.CheckLikedResp{IsLiked: record.Status == model.StatusActive}, nil
}
