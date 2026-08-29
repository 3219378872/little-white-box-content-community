package logic

import (
	"context"

	"esx/app/assistant/internal/runtime"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ConfirmRunToolLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewConfirmRunToolLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ConfirmRunToolLogic {
	return &ConfirmRunToolLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

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
