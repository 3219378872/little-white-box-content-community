package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// ListMemoryLogic 承载 ListMemory 接口的业务逻辑；每个请求新建一个实例。
type ListMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewListMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewListMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMemoryLogic {
	return &ListMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// ListMemory 列出记忆条目与容量占用。
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
