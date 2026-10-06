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

// UndoAssistantMemoryChangeLogic 承载 UndoAssistantMemoryChange 接口的业务逻辑；每个请求新建一个实例。
type UndoAssistantMemoryChangeLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUndoAssistantMemoryChangeLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUndoAssistantMemoryChangeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UndoAssistantMemoryChangeLogic {
	return &UndoAssistantMemoryChangeLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// UndoAssistantMemoryChange 按 ChangeId 撤销一次记忆变更，返回恢复后的条目。
func (l *UndoAssistantMemoryChangeLogic) UndoAssistantMemoryChange(req *types.UndoAssistantMemoryChangeReq) (*types.UndoAssistantMemoryChangeResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.Id <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	result, err := l.svcCtx.AssistantService.UndoMemoryChange(l.ctx, &assistantservice.UndoMemoryChangeReq{UserId: userID, ChangeId: req.Id})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	return &types.UndoAssistantMemoryChangeResp{Entry: mapMemory(result.GetEntry())}, nil
}
