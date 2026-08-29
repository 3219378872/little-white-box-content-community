package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAssistantMemoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAssistantMemoryLogic {
	return &ListAssistantMemoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAssistantMemoryLogic) ListAssistantMemory(req *types.ListAssistantMemoryReq) (*types.ListAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
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
