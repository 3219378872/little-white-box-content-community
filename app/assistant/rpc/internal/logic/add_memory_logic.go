package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// AddMemoryLogic 承载 AddMemory 接口的业务逻辑；每个请求新建一个实例。
type AddMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewAddMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewAddMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddMemoryLogic {
	return &AddMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// AddMemory 新增一条记忆并返回变更 ID。
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
