package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAddMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddMemoryLogic {
	return &AddMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *AddMemoryLogic) AddMemory(in *pb.AddMemoryReq) (*pb.AddMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	entry, changeID, err := l.svcCtx.Memory.Add(l.ctx, in.UserId, in.Target, in.Content, in.RequestId, store.NowMs())
	if err != nil {
		return nil, err
	}
	return &pb.AddMemoryResp{Entry: toPBMemory(entry), ChangeId: changeID}, nil
}
