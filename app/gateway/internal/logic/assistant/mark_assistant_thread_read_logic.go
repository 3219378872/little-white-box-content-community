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

// MarkAssistantThreadReadLogic 承载 MarkAssistantThreadRead 接口的业务逻辑；每个请求新建一个实例。
type MarkAssistantThreadReadLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMarkAssistantThreadReadLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewMarkAssistantThreadReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkAssistantThreadReadLogic {
	return &MarkAssistantThreadReadLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// MarkAssistantThreadRead 将助手会话标记为已读并返回剩余未读数。
func (l *MarkAssistantThreadReadLogic) MarkAssistantThreadRead() (*types.MarkAssistantThreadReadResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	result, err := l.svcCtx.AssistantService.MarkThreadRead(l.ctx, &assistantservice.MarkThreadReadReq{UserId: userID})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.MarkAssistantThreadReadResp{UnreadCount: result.GetUnreadCount()}, nil
}
