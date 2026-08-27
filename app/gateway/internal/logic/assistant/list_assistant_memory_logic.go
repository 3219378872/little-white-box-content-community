// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

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
	return &ListAssistantMemoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListAssistantMemoryLogic) ListAssistantMemory(req *types.ListAssistantMemoryReq) (*types.ListAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	layer := ""
	if req != nil {
		layer = req.Layer
	}
	result, err := l.svcCtx.AssistantService.ListMemory(l.ctx, &assistantservice.ListMemoryReq{
		UserId: userID,
		Layer:  layer,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	items := make([]types.AssistantMemoryItem, 0, len(result.GetItems()))
	for _, item := range result.GetItems() {
		if item == nil {
			continue
		}
		items = append(items, types.AssistantMemoryItem{
			Id:         item.Id,
			Layer:      item.Layer,
			Dimension:  item.Dimension,
			Value:      item.Value,
			Score:      item.Score,
			Source:     item.Source,
			Confidence: item.Confidence,
			Confirmed:  item.Confirmed,
			Suppressed: item.Suppressed,
			UpdatedAt:  item.UpdatedAt,
		})
	}
	return &types.ListAssistantMemoryResp{Items: items}, nil
}
