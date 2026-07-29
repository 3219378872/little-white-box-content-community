package logic

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"errx"
	"esx/app/assistant/rpc/internal/llm"
	"esx/app/assistant/rpc/internal/safety"
	"esx/app/assistant/rpc/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	defaultMaxMessageRunes = 2000
	defaultTokenChunkRunes = 64
	defaultToolTimeout     = 1500 * time.Millisecond
)

var blockedDirectives = []string{
	"ignore previous instructions",
	"ignore all instructions",
	"system prompt",
	"developer message",
	"忽略之前",
	"忽略以上",
	"系统提示词",
}

type ChatLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewChatLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ChatLogic {
	return &ChatLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ChatLogic) Chat(in *pb.ChatReq, stream pb.AssistantService_ChatServer) error {
	startedAt := time.Now()
	request, conversationID, err := l.validate(in)
	if err != nil {
		return err
	}
	if stream == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	if l.svcCtx == nil || l.svcCtx.Tools == nil {
		return l.sendDegraded(stream, conversationID, "ASSISTANT_UNAVAILABLE")
	}
	if l.svcCtx.Safety != nil {
		if err := l.svcCtx.Safety.Check(l.ctx, request.Message); err != nil {
			if errors.Is(err, safety.ErrBlocked) {
				return errx.New(errx.PermissionDenied, "request rejected by assistant content safety policy")
			}
			return err
		}
	}

	name, err := planTool(request.Message, &request)
	if err != nil {
		return err
	}
	if err := l.beginRequest(request, conversationID); err != nil {
		return err
	}
	toolCtx, cancel := context.WithTimeout(l.ctx, l.toolTimeout())
	defer cancel()

	result, err := l.svcCtx.Tools.Execute(toolCtx, name, request)
	if err != nil {
		assistantToolCallsTotal.Inc(string(name), toolCallMetricOutcome(err, toolCtx.Err()))
		if l.ctx.Err() != nil {
			return l.ctx.Err()
		}
		l.Errorw("assistant tool failed", logx.Field("tool", string(name)), logx.Field("err", err))
		code := toolErrorCode(err)
		if errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
			code = "TOOL_TIMEOUT"
		}
		return l.sendPersistedDegraded(stream, request, conversationID, code)
	}
	if result == nil || strings.TrimSpace(result.Text) == "" {
		assistantToolCallsTotal.Inc(string(name), "empty_result")
		l.Errorw("assistant tool returned an empty result", logx.Field("tool", string(name)))
		return l.sendPersistedDegraded(stream, request, conversationID, "EMPTY_TOOL_RESULT")
	}
	assistantToolCallsTotal.Inc(string(name), "success")
	responseText := result.Text
	if l.svcCtx.Generator != nil {
		generated, generateErr := l.svcCtx.Generator.Generate(l.ctx, llm.Request{
			UserMessage: request.Message, ToolName: string(name), ToolResult: result.Text,
		})
		if generateErr != nil {
			assistantLLMCallsTotal.Inc("failure")
			l.Errorw("assistant LLM failed", logx.Field("err", generateErr.Error()))
			return l.sendPersistedDegraded(stream, request, conversationID, "LLM_UNAVAILABLE")
		}
		assistantLLMCallsTotal.Inc("success")
		observeLLMUsage(generated)
		responseText = generated.Text
	}
	if l.svcCtx.Safety != nil {
		if err := l.svcCtx.Safety.Check(l.ctx, responseText); err != nil {
			if errors.Is(err, safety.ErrBlocked) {
				return l.sendPersistedDegraded(stream, request, conversationID, "CONTENT_FILTERED")
			}
			return err
		}
	}
	if err := l.persistAssistant(request, conversationID, responseText, result.Sources); err != nil {
		l.Errorw("assistant response persistence failed", logx.Field("err", err.Error()))
		return l.sendDegraded(stream, conversationID, "STATE_UNAVAILABLE")
	}

	for index, chunk := range splitRunes(responseText, l.tokenChunkRunes()) {
		if err := l.send(stream, &pb.ChatEvent{
			Type:           pb.ChatEventType_CHAT_EVENT_TYPE_TOKEN,
			Text:           chunk,
			ConversationId: conversationID,
		}); err != nil {
			return err
		}
		if index == 0 {
			assistantFirstTokenSeconds.ObserveFloat(time.Since(startedAt).Seconds())
		}
	}
	for _, source := range result.Sources {
		if source.Type == "" || source.ID == "" {
			continue
		}
		if err := l.send(stream, &pb.ChatEvent{
			Type: pb.ChatEventType_CHAT_EVENT_TYPE_SOURCE,
			Source: &pb.SourceReference{
				SourceType: source.Type,
				SourceId:   source.ID,
				Title:      source.Title,
			},
			ConversationId: conversationID,
		}); err != nil {
			return err
		}
	}

	return l.send(stream, &pb.ChatEvent{
		Type:           pb.ChatEventType_CHAT_EVENT_TYPE_DONE,
		ConversationId: conversationID,
	})
}

func (l *ChatLogic) beginRequest(request tool.Request, conversationID string) error {
	if l.svcCtx.Quota != nil {
		allowed, err := l.svcCtx.Quota.Allow(l.ctx, request.UserID)
		if err != nil {
			l.Errorw("assistant quota store failed", logx.Field("err", err.Error()))
			return errx.Wrap(err, errx.ServiceUnavailable)
		}
		if !allowed {
			return errx.NewWithCode(errx.TooManyReq)
		}
	}
	if l.svcCtx.Conversations == nil {
		return nil
	}
	err := l.svcCtx.Conversations.Append(l.ctx, request.UserID, conversationID, store.Message{
		Role: "user", Content: request.Message, RequestID: request.RequestID,
	})
	if errors.Is(err, store.ErrConversationOwnedByAnother) {
		return errx.New(errx.PermissionDenied, "conversation belongs to another user")
	}
	if err != nil {
		l.Errorw("assistant request persistence failed", logx.Field("err", err.Error()))
		return errx.Wrap(err, errx.ServiceUnavailable)
	}
	return nil
}

func (l *ChatLogic) persistAssistant(
	request tool.Request,
	conversationID string,
	text string,
	sources []tool.Source,
) error {
	if l.svcCtx.Conversations == nil {
		return nil
	}
	references := make([]store.Reference, 0, len(sources))
	for _, source := range sources {
		if source.Type == "" || source.ID == "" {
			continue
		}
		references = append(references, store.Reference{Type: source.Type, ID: source.ID, Title: source.Title})
	}
	return l.svcCtx.Conversations.Append(l.ctx, request.UserID, conversationID, store.Message{
		Role: "assistant", Content: text, RequestID: request.RequestID, Sources: references,
	})
}

func (l *ChatLogic) sendPersistedDegraded(
	stream pb.AssistantService_ChatServer,
	request tool.Request,
	conversationID string,
	code string,
) error {
	const message = "The assistant is temporarily unable to complete this request. Please try again later."
	if err := l.persistAssistant(request, conversationID, message, nil); err != nil {
		l.Errorw("assistant degradation persistence failed", logx.Field("err", err.Error()))
		code = "STATE_UNAVAILABLE"
	}
	return l.sendDegradedWithText(stream, conversationID, code, message)
}

func (l *ChatLogic) validate(in *pb.ChatReq) (tool.Request, string, error) {
	if in == nil {
		return tool.Request{}, "", errx.New(errx.ParamError, "chat request is required")
	}
	if in.UserId <= 0 {
		return tool.Request{}, "", errx.NewWithCode(errx.LoginRequired)
	}
	message := strings.TrimSpace(in.Message)
	if message == "" {
		return tool.Request{}, "", errx.New(errx.ParamError, "message is required")
	}
	if len([]rune(message)) > l.maxMessageRunes() {
		return tool.Request{}, "", errx.New(errx.ParamError, "message is too long")
	}
	for _, value := range message {
		if unicode.IsControl(value) && value != '\n' && value != '\r' && value != '\t' {
			return tool.Request{}, "", errx.New(errx.ParamError, "message contains unsupported control characters")
		}
	}
	lowerMessage := strings.ToLower(message)
	for _, directive := range blockedDirectives {
		if strings.Contains(lowerMessage, directive) {
			return tool.Request{}, "", errx.New(errx.PermissionDenied, "request rejected by assistant safety policy")
		}
	}

	conversationID := strings.TrimSpace(in.ConversationId)
	if conversationID == "" {
		conversationID = uuid.NewString()
	} else if !validOpaqueID(conversationID) {
		return tool.Request{}, "", errx.New(errx.ParamError, "invalid conversation id")
	}
	requestID := strings.TrimSpace(in.RequestId)
	if requestID == "" {
		requestID = uuid.NewString()
	} else if !validOpaqueID(requestID) {
		return tool.Request{}, "", errx.New(errx.ParamError, "invalid request id")
	}

	return tool.Request{
		UserID:    in.UserId,
		Message:   message,
		RequestID: requestID,
	}, conversationID, nil
}

func planTool(message string, request *tool.Request) (tool.Name, error) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "profile" || normalized == "my profile" || normalized == "我的资料" || normalized == "我是谁" {
		return tool.User, nil
	}
	if strings.Contains(normalized, "recommend") || strings.Contains(normalized, "推荐") {
		return tool.Recommend, nil
	}
	for _, prefix := range []string{"post:", "post ", "/post ", "帖子 "} {
		if !strings.HasPrefix(normalized, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(normalized, prefix))
		postID, err := strconv.ParseInt(value, 10, 64)
		if err != nil || postID <= 0 {
			return "", errx.New(errx.ParamError, "invalid post id")
		}
		request.PostID = postID
		return tool.Content, nil
	}
	return tool.Search, nil
}

func (l *ChatLogic) sendDegraded(stream pb.AssistantService_ChatServer, conversationID, code string) error {
	return l.sendDegradedWithText(stream, conversationID, code,
		"The assistant is temporarily unable to complete this request. Please try again later.")
}

func (l *ChatLogic) sendDegradedWithText(stream pb.AssistantService_ChatServer, conversationID, code, text string) error {
	return l.send(stream, &pb.ChatEvent{
		Type:           pb.ChatEventType_CHAT_EVENT_TYPE_ERROR,
		Text:           text,
		Degraded:       true,
		ErrorCode:      code,
		ConversationId: conversationID,
	})
}

func (l *ChatLogic) send(stream pb.AssistantService_ChatServer, event *pb.ChatEvent) error {
	if err := l.ctx.Err(); err != nil {
		return err
	}
	return stream.Send(event)
}

func toolErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "TOOL_TIMEOUT"
	case errx.Is(err, errx.PermissionDenied):
		return "TOOL_NOT_ALLOWED"
	case errx.Is(err, errx.ParamError):
		return "TOOL_REQUEST_INVALID"
	default:
		return "TOOL_UNAVAILABLE"
	}
}

func validOpaqueID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, current := range value {
		if current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' || current >= '0' && current <= '9' {
			continue
		}
		switch current {
		case '-', '_', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func splitRunes(value string, size int) []string {
	if size <= 0 {
		size = defaultTokenChunkRunes
	}
	runes := []rune(value)
	chunks := make([]string, 0, (len(runes)+size-1)/size)
	for len(runes) > 0 {
		end := min(size, len(runes))
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}

func (l *ChatLogic) maxMessageRunes() int {
	if l.svcCtx != nil && l.svcCtx.Config.MaxMessageRunes > 0 {
		return l.svcCtx.Config.MaxMessageRunes
	}
	return defaultMaxMessageRunes
}

func (l *ChatLogic) tokenChunkRunes() int {
	if l.svcCtx != nil && l.svcCtx.Config.TokenChunkRunes > 0 {
		return l.svcCtx.Config.TokenChunkRunes
	}
	return defaultTokenChunkRunes
}

func (l *ChatLogic) toolTimeout() time.Duration {
	if l.svcCtx != nil && l.svcCtx.Config.ToolTimeoutMs > 0 {
		return time.Duration(l.svcCtx.Config.ToolTimeoutMs) * time.Millisecond
	}
	return defaultToolTimeout
}
