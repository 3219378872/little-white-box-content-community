package logic

import (
	"context"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchMemoryLogic {
	return &BatchMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *BatchMemoryLogic) BatchMemory(in *pb.BatchMemoryReq) (*pb.BatchMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	ops := make([]memory.Op, 0, len(in.Ops))
	for _, op := range in.Ops {
		if op == nil {
			continue
		}
		ops = append(ops, memory.Op{Op: op.Op, ID: op.Id, Target: op.Target, Content: op.Content, Version: op.Version})
	}
	entries, ids, err := l.svcCtx.Memory.Batch(l.ctx, in.UserId, in.RequestId, ops, store.NowMs())
	if err != nil {
		return nil, err
	}
	out := make([]*pb.MemoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, toPBMemory(entry))
	}
	return &pb.BatchMemoryResp{Entries: out, ChangeIds: ids}, nil
}
