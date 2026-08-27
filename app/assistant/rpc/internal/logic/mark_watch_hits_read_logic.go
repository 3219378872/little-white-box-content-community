package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type MarkWatchHitsReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewMarkWatchHitsReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkWatchHitsReadLogic {
	return &MarkWatchHitsReadLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MarkWatchHitsReadLogic) MarkWatchHitsRead(in *pb.MarkWatchHitsReadReq) (*pb.MarkWatchHitsReadResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Watch == nil {
		return nil, unavailableUntilStore()
	}
	if err := l.svcCtx.Watch.MarkRead(l.ctx, in.UserId, in.HitIds); err != nil {
		return nil, err
	}
	return &pb.MarkWatchHitsReadResp{}, nil
}
