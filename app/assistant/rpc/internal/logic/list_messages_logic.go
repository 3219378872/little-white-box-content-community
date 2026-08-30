package logic

import (
	"context"

	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/pkg/errx"

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
	if in.BeforeId > 0 && in.AfterId > 0 {
		return nil, errx.New(errx.ParamError, "before_id and after_id are mutually exclusive")
	}
	limit := int(in.Limit)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	msgs, err := l.svcCtx.Store.ListMessages(l.ctx, in.UserId, in.SessionId, in.BeforeId, in.AfterId, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(msgs) > limit
	if hasMore {
		if in.AfterId > 0 {
			msgs = msgs[:limit]
		} else {
			msgs = msgs[len(msgs)-limit:]
		}
	}
	out := make([]*pb.AssistantMessage, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, &pb.AssistantMessage{
			Id: msg.ID, SessionId: msg.SessionID, RunId: msg.RunID, Role: msg.Role, Kind: msg.Kind,
			Content: msg.Content, Unread: msg.Unread, CreatedAtMs: msg.CreatedAtMs, ChangeId: msg.ChangeID,
		})
	}
	nextBeforeID := int64(0)
	if in.AfterId == 0 && len(msgs) > 0 {
		nextBeforeID = msgs[0].ID
	}
	return &pb.ListMessagesResp{Messages: out, HasMore: hasMore, NextBeforeId: nextBeforeID}, nil
}
