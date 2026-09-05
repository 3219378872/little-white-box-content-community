package logic

import (
	"context"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CancelRunLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCancelRunLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelRunLogic {
	return &CancelRunLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

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
	if err := runtime.ResolveWaiting(l.ctx, l.svcCtx.Store, l.svcCtx.Notify, in.RunId, store.NowMs()); err != nil {
		return nil, err
	}
	if l.svcCtx.Notify != nil {
		_ = l.svcCtx.Notify.Wake(l.ctx, in.RunId)
	}
	return &pb.CancelRunResp{}, nil
}
