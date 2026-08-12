// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package assistant

import (
	"context"
	"io"
	"strings"

	"errx"
	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const gatewayMaxMessageRunes = 2000

type AssistantChatLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// Assistant SSE 对话
func NewAssistantChatLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssistantChatLogic {
	return &AssistantChatLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssistantChatLogic) AssistantChat(req *types.AssistantChatReq, client chan<- *types.AssistantChatEvent) error {
	if client == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	conversationID := ""
	if req != nil {
		conversationID = strings.TrimSpace(req.ConversationId)
	}
	if req == nil || strings.TrimSpace(req.Message) == "" || len([]rune(strings.TrimSpace(req.Message))) > gatewayMaxMessageRunes {
		return l.sendError(client, conversationID, "INVALID_REQUEST", "A non-empty message of at most 2000 characters is required.")
	}

	userID, err := jwtx.GetUserIdFromContext(l.ctx)
	if err != nil || userID <= 0 {
		return l.sendError(client, conversationID, "AUTH_REQUIRED", "Authentication is required.")
	}
	if l.svcCtx == nil || l.svcCtx.AssistantService == nil {
		return l.sendError(client, conversationID, "ASSISTANT_UNAVAILABLE", "The assistant is temporarily unavailable.")
	}

	stream, err := l.svcCtx.AssistantService.Chat(l.ctx, &assistantservice.ChatReq{
		UserId:         userID,
		ConversationId: conversationID,
		Message:        strings.TrimSpace(req.Message),
		RequestId:      strings.TrimSpace(req.RequestId),
	})
	if err != nil {
		l.Errorw("assistant stream start failed", logx.Field("err", err))
		return l.sendRPCError(client, conversationID, err)
	}

	seenEvent := false
	terminalEvent := false
	for {
		event, recvErr := stream.Recv()
		if recvErr == io.EOF {
			switch {
			case terminalEvent:
				return nil
			case seenEvent:
				return l.sendError(client, conversationID, "STREAM_INCOMPLETE", "The assistant response ended before completion.")
			default:
				return l.sendError(client, conversationID, "EMPTY_STREAM", "The assistant returned no response.")
			}
		}
		if recvErr != nil {
			if l.ctx.Err() != nil {
				return l.ctx.Err()
			}
			if terminalEvent {
				return nil
			}
			l.Errorw("assistant stream receive failed", logx.Field("err", recvErr))
			return l.sendRPCError(client, conversationID, recvErr)
		}
		if event == nil {
			return l.sendError(client, conversationID, "INVALID_STREAM_EVENT", "The assistant returned an invalid response.")
		}
		if event.ConversationId != "" {
			conversationID = event.ConversationId
		}
		mapped, terminal, ok := mapChatEvent(event, conversationID)
		if !ok {
			return l.sendError(client, conversationID, "INVALID_STREAM_EVENT", "The assistant returned an invalid response.")
		}
		if err := l.send(client, mapped); err != nil {
			return err
		}
		seenEvent = true
		terminalEvent = terminalEvent || terminal
	}
}

func mapChatEvent(event *assistantpb.ChatEvent, conversationID string) (*types.AssistantChatEvent, bool, bool) {
	mapped := &types.AssistantChatEvent{
		Text:           event.Text,
		Degraded:       event.Degraded,
		ErrorCode:      event.ErrorCode,
		ConversationId: conversationID,
	}
	terminal := false
	switch event.Type {
	case assistantpb.ChatEventType_CHAT_EVENT_TYPE_TOKEN:
		if event.Text == "" {
			return nil, false, false
		}
		mapped.Type = "token"
	case assistantpb.ChatEventType_CHAT_EVENT_TYPE_SOURCE:
		if event.Source == nil || event.Source.SourceType == "" || event.Source.SourceId == "" {
			return nil, false, false
		}
		mapped.Type = "source"
		mapped.Source = &types.AssistantSourceReference{
			SourceType: event.Source.SourceType,
			SourceId:   event.Source.SourceId,
			Title:      event.Source.Title,
			Revision:   event.Source.Revision,
		}
	case assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE:
		mapped.Type = "done"
		terminal = true
	case assistantpb.ChatEventType_CHAT_EVENT_TYPE_ERROR:
		if event.ErrorCode == "" || event.Text == "" {
			return nil, false, false
		}
		mapped.Type = "error"
		mapped.Degraded = true
		terminal = true
	default:
		return nil, false, false
	}
	return mapped, terminal, true
}

func (l *AssistantChatLogic) sendRPCError(client chan<- *types.AssistantChatEvent, conversationID string, rpcErr error) error {
	code := "ASSISTANT_UNAVAILABLE"
	text := "The assistant is temporarily unavailable."
	switch status.Code(rpcErr) {
	case codes.InvalidArgument:
		code = "INVALID_REQUEST"
		text = "The assistant rejected this request."
	case codes.PermissionDenied:
		code = "REQUEST_REJECTED"
		text = "The assistant rejected this request."
	case codes.ResourceExhausted:
		code = "QUOTA_EXCEEDED"
		text = "The assistant request quota has been reached."
	case codes.DeadlineExceeded:
		code = "ASSISTANT_TIMEOUT"
		text = "The assistant did not respond in time."
	}
	return l.sendError(client, conversationID, code, text)
}

func (l *AssistantChatLogic) sendError(client chan<- *types.AssistantChatEvent, conversationID, code, text string) error {
	return l.send(client, &types.AssistantChatEvent{
		Type:           "error",
		Text:           text,
		Degraded:       true,
		ErrorCode:      code,
		ConversationId: conversationID,
	})
}

func (l *AssistantChatLogic) send(client chan<- *types.AssistantChatEvent, event *types.AssistantChatEvent) error {
	select {
	case <-l.ctx.Done():
		return l.ctx.Err()
	case client <- event:
		return nil
	}
}
