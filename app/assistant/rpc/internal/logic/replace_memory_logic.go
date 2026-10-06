package logic

import (
	"context"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// ReplaceMemoryLogic 承载 ReplaceMemory 接口的业务逻辑；每个请求新建一个实例。
type ReplaceMemoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewReplaceMemoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewReplaceMemoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReplaceMemoryLogic {
	return &ReplaceMemoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// ReplaceMemory 按版本替换一条记忆并返回变更 ID。
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
