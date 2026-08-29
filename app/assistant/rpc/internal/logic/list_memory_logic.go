package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMemoryLogic {
	return &ListMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ListMemoryLogic) ListMemory(in *pb.ListMemoryReq) (*pb.ListMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	items, caps, err := l.svcCtx.Memory.List(l.ctx, in.UserId, in.Target)
	if err != nil {
		return nil, err
	}
	out := make([]*pb.MemoryEntry, 0, len(items))
	for _, item := range items {
		out = append(out, toPBMemory(item))
	}
	return &pb.ListMemoryResp{Items: out, Capacities: toPBCapacities(caps)}, nil
}
