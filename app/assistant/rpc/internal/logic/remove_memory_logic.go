package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// RemoveMemoryLogic 承载 RemoveMemory 接口的业务逻辑；每个请求新建一个实例。
type RemoveMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewRemoveMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewRemoveMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveMemoryLogic {
	return &RemoveMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// RemoveMemory 按版本删除一条记忆并返回变更 ID。
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
