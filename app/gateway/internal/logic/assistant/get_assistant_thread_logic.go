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

// GetAssistantThreadLogic 承载 GetAssistantThread 接口的业务逻辑；每个请求新建一个实例。
type GetAssistantThreadLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewGetAssistantThreadLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetAssistantThreadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetAssistantThreadLogic {
	return &GetAssistantThreadLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// GetAssistantThread 返回当前用户的助手会话摘要（未读数、最近消息与进行中的运行）。
func (l *GetAssistantThreadLogic) GetAssistantThread() (*types.GetAssistantThreadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.AssistantService.GetThread(l.ctx, &assistantservice.GetThreadReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.GetAssistantThreadResp{Thread: mapThread(result.GetThread())}, nil
}
