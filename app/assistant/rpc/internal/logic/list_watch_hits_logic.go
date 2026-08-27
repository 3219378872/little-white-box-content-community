package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWatchHitsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListWatchHitsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWatchHitsLogic {
	return &ListWatchHitsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListWatchHitsLogic) ListWatchHits(in *pb.ListWatchHitsReq) (*pb.ListWatchHitsResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	return &pb.ListWatchHitsResp{Hits: []*pb.WatchHit{}}, nil
}
