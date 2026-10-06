package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

type RemoveMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

func NewRemoveMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveMemoryLogic {
	return &RemoveMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

func (l *RemoveMemoryLogic) RemoveMemory(in *pb.RemoveMemoryReq) (*pb.RemoveMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	changeID, err := l.svcCtx.Memory.Remove(l.ctx, in.UserId, in.Id, in.Version, in.RequestId, store.NowMs())
	if err != nil {
		return nil, err
	}
	return &pb.RemoveMemoryResp{ChangeId: changeID}, nil
}
