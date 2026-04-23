package logic

import (
	"context"

	"errx"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/validator"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchCheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckLikedLogic {
	return &BatchCheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *BatchCheckLikedLogic) BatchCheckLiked(in *pb.BatchCheckLikedReq) (*pb.BatchCheckLikedResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.TargetIds) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	results := make(map[int64]bool, len(in.TargetIds))
	if len(in.TargetIds) == 0 {
		return &pb.BatchCheckLikedResp{Results: results}, nil
	}

	statusMap, err := l.svcCtx.LikeRecordModel.FindStatusByUserAndTargets(l.ctx, in.UserId, in.TargetIds, int64(in.TargetType))
	if err != nil {
		l.Logger.Errorw("FindStatusByUserAndTargets failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	for _, targetID := range in.TargetIds {
		results[targetID] = statusMap[targetID]
	}

	return &pb.BatchCheckLikedResp{Results: results}, nil
}
