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

// DeleteAssistantHistoryLogic 承载 DeleteAssistantHistory 接口的业务逻辑；每个请求新建一个实例。
type DeleteAssistantHistoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteAssistantHistoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewDeleteAssistantHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteAssistantHistoryLogic {
	return &DeleteAssistantHistoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// DeleteAssistantHistory 清空当前用户的助手对话历史。
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
