package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMemoryLogic {
	return &ListMemoryLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ListMemoryLogic) ListMemory(in *pb.ListMemoryReq) (*pb.ListMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	return &pb.ListMemoryResp{Items: []*pb.MemoryItem{}}, nil
}
