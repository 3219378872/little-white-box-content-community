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

// CheckLikedLogic 承载 CheckLiked 接口的业务逻辑；每个请求新建一个实例。
type CheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewCheckLikedLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckLikedLogic {
	return &CheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// CheckLiked 查询用户当前是否点赞了该目标；取消点赞的记录视为未点赞。
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
