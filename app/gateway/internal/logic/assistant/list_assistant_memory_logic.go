package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// ListAssistantMemoryLogic 承载 ListAssistantMemory 接口的业务逻辑；每个请求新建一个实例。
type ListAssistantMemoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewListAssistantMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewListAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAssistantMemoryLogic {
	return &ListAssistantMemoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// ListAssistantMemory 列出记忆条目及各目标的容量占用；Target 为空时返回全部目标。
func (l *ListAssistantMemoryLogic) ListAssistantMemory(req *types.ListAssistantMemoryReq) (*types.ListAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	target := ""
	if req != nil {
		target = req.Target
	}
	result, err := l.svcCtx.AssistantService.ListMemory(l.ctx, &assistantservice.ListMemoryReq{UserId: userID, Target: target})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	items := make([]types.AssistantMemoryEntry, 0, len(result.GetItems()))
	for _, item := range result.GetItems() {
		items = append(items, mapMemory(item))
	}
	caps := make([]types.AssistantMemoryCapacity, 0, len(result.GetCapacities()))
	for _, cap := range result.GetCapacities() {
		if cap == nil {
			continue
		}
		caps = append(caps, types.AssistantMemoryCapacity{Target: cap.Target, Used: cap.Used, Limit: cap.Limit})
	}
	return &types.ListAssistantMemoryResp{Items: items, Capacities: caps}, nil
}
