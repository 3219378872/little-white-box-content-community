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

// BatchAssistantMemoryLogic 承载 BatchAssistantMemory 接口的业务逻辑；每个请求新建一个实例。
type BatchAssistantMemoryLogic struct {
	logging.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewBatchAssistantMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewBatchAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchAssistantMemoryLogic {
	return &BatchAssistantMemoryLogic{Logger: logging.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

// BatchAssistantMemory 一次提交多条记忆增删改，返回每条操作的 ChangeId 供撤销。
func (l *BatchAssistantMemoryLogic) BatchAssistantMemory(req *types.BatchAssistantMemoryReq) (*types.BatchAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || len(req.Ops) == 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	ops := make([]*assistantservice.MemoryOp, 0, len(req.Ops))
	for _, op := range req.Ops {
		ops = append(ops, &assistantservice.MemoryOp{Op: op.Op, Id: op.Id, Target: op.Target, Content: op.Content, Version: op.Version})
	}
	result, err := l.svcCtx.AssistantService.BatchMemory(l.ctx, &assistantservice.BatchMemoryReq{
		UserId: userID, RequestId: req.RequestId, Ops: ops,
	})
	if err != nil {
		return nil, errx.FromRPCError(err)
	}
	entries := make([]types.AssistantMemoryEntry, 0, len(result.GetEntries()))
	for _, item := range result.GetEntries() {
		entries = append(entries, mapMemory(item))
	}
	return &types.BatchAssistantMemoryResp{Entries: entries, ChangeIds: result.GetChangeIds()}, nil
}
