package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// DeleteHistoryLogic 承载 DeleteHistory 接口的业务逻辑；每个请求新建一个实例。
type DeleteHistoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewDeleteHistoryLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewDeleteHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteHistoryLogic {
	return &DeleteHistoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// DeleteHistory 删除用户全部助手历史。
func (l *DeleteHistoryLogic) DeleteHistory(in *pb.DeleteHistoryReq) (*pb.DeleteHistoryResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Acceptor == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	if err := l.svcCtx.Acceptor.DeleteHistory(l.ctx, in.UserId); err != nil {
		return nil, err
	}
	return &pb.DeleteHistoryResp{}, nil
}
