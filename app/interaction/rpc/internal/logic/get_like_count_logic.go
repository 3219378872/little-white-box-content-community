package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"

	"esx/pkg/logging"
)

// GetLikeCountLogic 承载 GetLikeCount 接口的业务逻辑；每个请求新建一个实例。
type GetLikeCountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetLikeCountLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetLikeCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikeCountLogic {
	return &GetLikeCountLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetLikeCount 复用 GetCounts 的缓存路径，只返回点赞数。
func (l *GetLikeCountLogic) GetLikeCount(in *pb.GetLikeCountReq) (*pb.GetLikeCountResp, error) {
	resp, err := NewGetCountsLogic(l.ctx, l.svcCtx).GetCounts(&pb.GetCountsReq{
		TargetId:   in.TargetId,
		TargetType: in.TargetType,
	})
	if err != nil {
		return nil, err
	}

	return &pb.GetLikeCountResp{Count: resp.LikeCount}, nil
}
