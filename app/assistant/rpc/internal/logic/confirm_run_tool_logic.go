package logic

import (
	"context"

	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// ConfirmRunToolLogic 承载 ConfirmRunTool 接口的业务逻辑；每个请求新建一个实例。
type ConfirmRunToolLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewConfirmRunToolLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewConfirmRunToolLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmRunToolLogic {
	return &ConfirmRunToolLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// ConfirmRunTool 提交用户对待确认工具调用的决定。
func (l *ConfirmRunToolLogic) ConfirmRunTool(in *pb.ConfirmRunToolReq) (*pb.ConfirmRunToolResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	if err := runtime.Confirm(l.ctx, l.svcCtx.Store, in.UserId, in.RunId, in.CallId, in.Approved); err != nil {
		return nil, err
	}
	return &pb.ConfirmRunToolResp{}, nil
}
