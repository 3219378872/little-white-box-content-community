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

type AddAssistantMemoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddAssistantMemoryLogic {
	return &AddAssistantMemoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *AddAssistantMemoryLogic) AddAssistantMemory(req *types.AddAssistantMemoryReq) (*types.AddAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if req == nil || strings.TrimSpace(req.Target) == "" || strings.TrimSpace(req.Content) == "" {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.AddMemory(l.ctx, &assistantservice.AddMemoryReq{
		UserId: userID, Target: req.Target, Content: req.Content, RequestId: req.RequestId,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.AddAssistantMemoryResp{Entry: mapMemory(result.GetEntry()), ChangeId: result.GetChangeId()}, nil
}
