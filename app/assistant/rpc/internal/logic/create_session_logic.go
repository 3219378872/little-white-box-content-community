package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateSessionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCreateSessionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateSessionLogic {
	return &CreateSessionLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CreateSessionLogic) CreateSession(in *pb.CreateSessionReq) (*pb.CreateSessionResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Acceptor == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	id, err := l.svcCtx.Acceptor.CreateSession(l.ctx, in.UserId)
	if err != nil {
		return nil, err
	}
	return &pb.CreateSessionResp{SessionId: id}, nil
}
