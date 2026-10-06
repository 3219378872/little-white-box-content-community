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

// RemoveAssistantMemoryLogic 承载 RemoveAssistantMemory 接口的业务逻辑；每个请求新建一个实例。
type RemoveAssistantMemoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewRemoveAssistantMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewRemoveAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveAssistantMemoryLogic {
	return &RemoveAssistantMemoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// RemoveAssistantMemory 删除一条记忆；携带版本号做乐观并发控制。
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
