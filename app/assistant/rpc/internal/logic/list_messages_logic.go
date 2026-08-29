package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMessagesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewListMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMessagesLogic {
	return &ListMessagesLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *ListMessagesLogic) ListMessages(in *pb.ListMessagesReq) (*pb.ListMessagesResp, error) {
	if in == nil {
		return nil, requireAgentUser(0)
	}
	if err := requireAgentUser(in.UserId); err != nil {
		return nil, err
	}
	if l.svcCtx == nil || l.svcCtx.Store == nil {
		return nil, unavailableUntilStore()
	}
	msgs, err := l.svcCtx.Store.ListMessages(l.ctx, in.UserId, in.SessionId, in.AfterId, int(in.Limit))
	if err != nil {
		return nil, err
	}
	out := make([]*pb.AssistantMessage, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, &pb.AssistantMessage{
			Id: msg.ID, SessionId: msg.SessionID, RunId: msg.RunID, Role: msg.Role, Kind: msg.Kind,
			Content: msg.Content, Unread: msg.Unread, CreatedAtMs: msg.CreatedAtMs, ChangeId: msg.ChangeID,
		})
	}
	return &pb.ListMessagesResp{Messages: out}, nil
}
