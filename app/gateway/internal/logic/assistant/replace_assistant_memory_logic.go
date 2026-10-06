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

// ReplaceAssistantMemoryLogic 承载 ReplaceAssistantMemory 接口的业务逻辑；每个请求新建一个实例。
type ReplaceAssistantMemoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReplaceAssistantMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewReplaceAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReplaceAssistantMemoryLogic {
	return &ReplaceAssistantMemoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// ReplaceAssistantMemory 整体替换一条记忆的内容；携带版本号做乐观并发控制。
func (l *ReplaceAssistantMemoryLogic) ReplaceAssistantMemory(req *types.ReplaceAssistantMemoryReq) (*types.ReplaceAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
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
