package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReplaceMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewReplaceMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReplaceMemoryLogic {
	return &ReplaceMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ReplaceMemoryLogic) ReplaceMemory(in *pb.ReplaceMemoryReq) (*pb.ReplaceMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return nil, unavailableUntilStore()
	}
	entry, changeID, err := l.svcCtx.Memory.Replace(l.ctx, in.UserId, in.Id, in.Content, in.Version, in.RequestId, store.NowMs())
	if err != nil {
		return nil, err
	}
	return &pb.ReplaceMemoryResp{Entry: toPBMemory(entry), ChangeId: changeID}, nil
}
