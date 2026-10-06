package assistant

import (
	"context"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	logx "esx/pkg/logging"
)

type RemoveAssistantMemoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveAssistantMemoryLogic {
	return &RemoveAssistantMemoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RemoveAssistantMemoryLogic) RemoveAssistantMemory(req *types.RemoveAssistantMemoryReq) (*types.RemoveAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.RemoveMemory(l.ctx, &assistantservice.RemoveMemoryReq{
		UserId: userID, Id: req.Id, Version: req.Version, RequestId: req.RequestId,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.RemoveAssistantMemoryResp{ChangeId: result.GetChangeId()}, nil
}
