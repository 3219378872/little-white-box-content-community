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

type DeleteAssistantHistoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteAssistantHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteAssistantHistoryLogic {
	return &DeleteAssistantHistoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteAssistantHistoryLogic) DeleteAssistantHistory() (*types.DeleteAssistantHistoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if _, err := l.svcCtx.AssistantService.DeleteHistory(l.ctx, &assistantservice.DeleteHistoryReq{UserId: userID}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.DeleteAssistantHistoryResp{}, nil
}
