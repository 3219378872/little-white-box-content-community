package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type UndoMemoryChangeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUndoMemoryChangeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UndoMemoryChangeLogic {
	return &UndoMemoryChangeLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

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
