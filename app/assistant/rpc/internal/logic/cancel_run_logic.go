package logic

import (
	"context"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// CancelRunLogic 承载 CancelRun 接口的业务逻辑；每个请求新建一个实例。
type CancelRunLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewCancelRunLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewCancelRunLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelRunLogic {
	return &CancelRunLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// CancelRun 标记取消；等待追问的 run 没有 worker 在跑，需要在这里直接结算。
func (l *CancelRunLogic) CancelRun(in *pb.CancelRunReq) (*pb.CancelRunResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	if err := l.svcCtx.Store.RequestCancel(l.ctx, in.UserId, in.RunId); err != nil {
		return nil, err
	}
	if err := runtime.ResolveWaiting(l.ctx, l.svcCtx.Store, in.RunId, store.NowMs()); err != nil {
		return nil, err
	}
	return &pb.CancelRunResp{}, nil
}
