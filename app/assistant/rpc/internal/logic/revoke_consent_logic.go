package logic

import (
	"context"
	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/internal/store"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type RevokeConsentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRevokeConsentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RevokeConsentLogic {
	return &RevokeConsentLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

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
	if err := l.svcCtx.Store.ResetUnsentBuckets(l.ctx, in.UserId); err != nil {
		return nil, err
	}
	return &pb.RevokeConsentResp{}, nil
}
