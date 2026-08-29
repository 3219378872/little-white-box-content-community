package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteHistoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewDeleteHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteHistoryLogic {
	return &DeleteHistoryLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

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
