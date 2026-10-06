package logic

import (
	"context"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"

	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"

	"esx/pkg/logging"
)

// RevokeConsentLogic 承载 RevokeConsent 接口的业务逻辑；每个请求新建一个实例。
type RevokeConsentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewRevokeConsentLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewRevokeConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeConsentLogic {
	return &RevokeConsentLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// RevokeConsent 取消用户全部未结束的 run，并立即结算正在等待追问的 run。
func (l *RevokeConsentLogic) RevokeConsent(in *pb.RevokeConsentReq) (*pb.RevokeConsentResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	if err := l.svcCtx.Store.RequestCancelAll(l.ctx, in.UserId); err != nil {
		return nil, err
	}
	if thread, err := l.svcCtx.Store.GetThread(l.ctx, in.UserId); err != nil {
		return nil, err
	} else if thread.ActiveRunID > 0 {
		if err := runtime.ResolveWaiting(l.ctx, l.svcCtx.Store, l.svcCtx.Notify, thread.ActiveRunID, store.NowMs()); err != nil {
			return nil, err
		}
	}
	return &pb.RevokeConsentResp{}, nil
}
