package assistant

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReplaceAssistantMemoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReplaceAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReplaceAssistantMemoryLogic {
	return &ReplaceAssistantMemoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ReplaceAssistantMemoryLogic) ReplaceAssistantMemory(req *types.ReplaceAssistantMemoryReq) (*types.ReplaceAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || req.Id <= 0 || strings.TrimSpace(req.Content) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.ReplaceMemory(l.ctx, &assistantservice.ReplaceMemoryReq{
		UserId: userID, Id: req.Id, Content: req.Content, Version: req.Version, RequestId: req.RequestId,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.ReplaceAssistantMemoryResp{Entry: mapMemory(result.GetEntry()), ChangeId: result.GetChangeId()}, nil
}
