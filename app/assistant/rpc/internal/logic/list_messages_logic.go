package logic

import (
	"context"
	"encoding/json"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"

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
		item := &pb.AssistantMessage{
			Id: msg.ID, SessionId: msg.SessionID, RunId: msg.RunID, Role: msg.Role, Kind: msg.Kind,
			Content: msg.Content, Unread: msg.Unread, CreatedAtMs: msg.CreatedAtMs, ChangeId: msg.ChangeID,
		}
		if msg.Kind == store.KindQuestion {
			questions, err := l.svcCtx.Store.ListQuestions(l.ctx, msg.RunID)
			if err != nil {
				return nil, err
			}
			for _, q := range questions {
				if q.MessageID == msg.ID {
					raw, err := json.Marshal(q)
					if err != nil {
						return nil, err
					}
					item.QuestionRequestJson = string(raw)
					break
				}
			}
		}
		if msg.Role == store.RoleAssistant && msg.Kind != store.KindQuestion {
			presentation, err := l.svcCtx.Store.GetPresentation(l.ctx, msg.ID)
			if err != nil {
				return nil, err
			}
			if presentation == nil {
				presentation, err = tool.LegacyPresentation(l.ctx, l.svcCtx.Store, msg)
				if err != nil {
					return nil, err
				}
			}
			if presentation != nil {
				tool.RevalidatePresentation(l.ctx, tool.Clients{Content: l.svcCtx.ContentService, Store: l.svcCtx.Store}, in.UserId, presentation)
				raw, err := json.Marshal(presentation)
				if err != nil {
					return nil, err
				}
				item.AnswerPresentationJson = string(raw)
			}
		}
		out = append(out, item)
	}
	nextBeforeID := int64(0)
	if in.AfterId == 0 && len(msgs) > 0 {
		nextBeforeID = msgs[0].ID
	}
	return &pb.ListMessagesResp{Messages: out, HasMore: hasMore, NextBeforeId: nextBeforeID}, nil
}
