package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// UndoMemoryChangeLogic 承载 UndoMemoryChange 接口的业务逻辑；每个请求新建一个实例。
type UndoMemoryChangeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewUndoMemoryChangeLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewUndoMemoryChangeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UndoMemoryChangeLogic {
	return &UndoMemoryChangeLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// UndoMemoryChange 撤销一次记忆变更。
func (l *UndoMemoryChangeLogic) UndoMemoryChange(in *pb.UndoMemoryChangeReq) (*pb.UndoMemoryChangeResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	entry, err := l.svcCtx.Memory.Undo(l.ctx, in.UserId, in.ChangeId, store.NowMs())
	if err != nil {
		return nil, err
	}
	resp := &pb.UndoMemoryChangeResp{}
	if entry != nil {
		resp.Entry = toPBMemory(*entry)
	}
	return resp, nil
}
