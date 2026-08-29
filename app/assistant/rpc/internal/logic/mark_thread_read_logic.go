package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type MarkThreadReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewMarkThreadReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkThreadReadLogic {
	return &MarkThreadReadLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MarkThreadReadLogic) MarkThreadRead(in *pb.MarkThreadReadReq) (*pb.MarkThreadReadResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Acceptor == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	unread, err := l.svcCtx.Acceptor.MarkRead(l.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	return &pb.MarkThreadReadResp{UnreadCount: unread}, nil
}
