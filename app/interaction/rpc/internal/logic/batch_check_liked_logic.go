package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/errx"
	"esx/pkg/validator"

	"esx/pkg/logging"
)

// BatchCheckLikedLogic 承载 BatchCheckLiked 接口的业务逻辑；每个请求新建一个实例。
type BatchCheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewBatchCheckLikedLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewBatchCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckLikedLogic {
	return &BatchCheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// BatchCheckLiked 批量查询用户是否点赞了各目标；未出现的目标返回 false。
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
		l.Errorw("FindStatusByUserAndTargets failed",
			logging.Field("userId", in.UserId),
			logging.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	for _, targetID := range in.TargetIds {
		results[targetID] = statusMap[targetID]
	}

	return &pb.BatchCheckLikedResp{Results: results}, nil
}
