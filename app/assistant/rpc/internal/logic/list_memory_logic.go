package logic

import (
	"context"
	"time"

	"esx/app/assistant/rpc/internal/memory"
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
	return &ListMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ListMemoryLogic) ListMemory(in *pb.ListMemoryReq) (*pb.ListMemoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Memory == nil {
		return &pb.ListMemoryResp{Items: []*pb.MemoryItem{}}, nil
	}
	items, err := l.svcCtx.Memory.List(l.ctx, in.UserId, in.Layer, time.Now())
	if err != nil {
		return nil, err
	}
	out := make([]*pb.MemoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, toPBMemory(item))
	}
	return &pb.ListMemoryResp{Items: out}, nil
}

func toPBMemory(item memory.Item) *pb.MemoryItem {
	return &pb.MemoryItem{
		Id: item.ID, Layer: item.Layer, Dimension: item.Dimension, Value: item.Value,
		Score: item.Score, Source: item.Source, Confidence: item.Confidence,
		Confirmed: item.Confirmed(), Suppressed: item.Suppressed, UpdatedAt: item.UpdatedAt,
	}
}
