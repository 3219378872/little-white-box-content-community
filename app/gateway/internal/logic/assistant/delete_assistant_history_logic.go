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

type DeleteAssistantHistoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteAssistantHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteAssistantHistoryLogic {
	return &DeleteAssistantHistoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteAssistantHistoryLogic) DeleteAssistantHistory() (*types.DeleteAssistantHistoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
	}
	if _, err := l.svcCtx.AssistantService.DeleteHistory(l.ctx, &assistantservice.DeleteHistoryReq{UserId: userID}); err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.DeleteAssistantHistoryResp{}, nil
}
