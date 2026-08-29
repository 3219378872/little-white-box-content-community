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

type BatchAssistantMemoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBatchAssistantMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchAssistantMemoryLogic {
	return &BatchAssistantMemoryLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *BatchAssistantMemoryLogic) BatchAssistantMemory(req *types.BatchAssistantMemoryReq) (*types.BatchAssistantMemoryResp, error) {
	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil {
		return nil, errx.NewWithCode(errx.LoginRequired)
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
