package assistant

import (
	"context"
	"strings"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/logging"
)

// AddAssistantMemoryLogic 承载 AddAssistantMemory 接口的业务逻辑；每个请求新建一个实例。
type AddAssistantMemoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewAddAssistantMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewAddAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddAssistantMemoryLogic {
	return &AddAssistantMemoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// AddAssistantMemory 为当前用户新增一条助手记忆；返回的 ChangeId 供前端提供撤销入口。
func (l *AddAssistantMemoryLogic) AddAssistantMemory(req *types.AddAssistantMemoryReq) (*types.AddAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
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
