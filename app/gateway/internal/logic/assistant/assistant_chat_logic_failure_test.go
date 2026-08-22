package assistant

import (
	"context"
	"testing"

	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAssistantChatNilClientChannel(t *testing.T) {
	t.Parallel()
	logic := NewAssistantChatLogic(jwtx.WithUserIdContext(context.Background(), 1), &svc.ServiceContext{})
	err := logic.AssistantChat(&types.AssistantChatReq{Message: "hello"}, nil)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ServiceUnavailable), "got %v", err)
}

func TestAssistantChatRequiresAuth(t *testing.T) {
	t.Parallel()
	events, err := collectEvents(t,
		NewAssistantChatLogic(context.Background(), &svc.ServiceContext{}),
		&types.AssistantChatReq{Message: "hello"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "AUTH_REQUIRED", events[0].ErrorCode)
}

func TestAssistantChatWithoutAssistantService(t *testing.T) {
	t.Parallel()
	events, err := collectEvents(t,
		NewAssistantChatLogic(jwtx.WithUserIdContext(context.Background(), 1), &svc.ServiceContext{}),
		&types.AssistantChatReq{Message: "hello"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "ASSISTANT_UNAVAILABLE", events[0].ErrorCode)
}

func TestAssistantChatStreamIncompleteWithoutTerminalEvent(t *testing.T) {
	t.Parallel()
	ctx := jwtx.WithUserIdContext(context.Background(), 1)
	service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		return &fakeAssistantChatClient{ctx: callCtx, events: []*assistantpb.ChatEvent{
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_TOKEN, Text: "partial"},
		}}, nil
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}),
		&types.AssistantChatReq{Message: "hello"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 1)
	assert.Equal(t, "STREAM_INCOMPLETE", events[len(events)-1].ErrorCode)
}

func TestAssistantChatRecvErrorAfterTerminalEventSucceedsQuietly(t *testing.T) {
	t.Parallel()
	ctx := jwtx.WithUserIdContext(context.Background(), 1)
	service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		return &fakeAssistantChatClient{
			ctx: callCtx,
			events: []*assistantpb.ChatEvent{
				{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE},
			},
			err: status.Error(codes.Internal, "post-terminal noise"),
		}, nil
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}),
		&types.AssistantChatReq{Message: "hello"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "done", events[0].Type)
}

func TestAssistantChatRecvFailureBeforeTerminalMapsRPCCode(t *testing.T) {
	for _, tc := range []struct {
		code codes.Code
		want string
	}{
		{codes.InvalidArgument, "INVALID_REQUEST"},
		{codes.PermissionDenied, "REQUEST_REJECTED"},
		{codes.ResourceExhausted, "QUOTA_EXCEEDED"},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			ctx := jwtx.WithUserIdContext(context.Background(), 1)
			service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
				return &fakeAssistantChatClient{ctx: callCtx, err: status.Error(tc.code, "mid-stream")}, nil
			}}
			events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}),
				&types.AssistantChatReq{Message: "hello"})
			require.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, tc.want, events[0].ErrorCode)
		})
	}
}

func TestAssistantChatInvalidStreamEventsAreStructuredErrors(t *testing.T) {
	for name, events := range map[string][]*assistantpb.ChatEvent{
		"nil event":            {nil},
		"empty token":          {{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_TOKEN, Text: ""}},
		"incomplete source":    {{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_SOURCE, Source: &assistantpb.SourceReference{SourceType: "post"}}},
		"error without fields": {{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_ERROR}},
		"unknown type":         {{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_UNSPECIFIED}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := jwtx.WithUserIdContext(context.Background(), 1)
			service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
				return &fakeAssistantChatClient{ctx: callCtx, events: events}, nil
			}}
			got, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}),
				&types.AssistantChatReq{Message: "hello"})
			require.NoError(t, err)
			require.NotEmpty(t, got)
			assert.Equal(t, "INVALID_STREAM_EVENT", got[len(got)-1].ErrorCode)
		})
	}
}

func TestAssistantChatErrorEventIsTerminalAndStructured(t *testing.T) {
	t.Parallel()
	ctx := jwtx.WithUserIdContext(context.Background(), 1)
	service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		return &fakeAssistantChatClient{ctx: callCtx, events: []*assistantpb.ChatEvent{
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_ERROR, ErrorCode: "SAFETY", Text: "rejected"},
		}}, nil
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}),
		&types.AssistantChatReq{Message: "hello"})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "error", events[0].Type)
	assert.True(t, events[0].Degraded)
	assert.Equal(t, "SAFETY", events[0].ErrorCode)
	assert.Equal(t, "rejected", events[0].Text)
}

func TestAssistantChatSendAbortsWhenContextAlreadyCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(jwtx.WithUserIdContext(context.Background(), 1))
	cancel()
	service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		return &fakeAssistantChatClient{ctx: callCtx, events: []*assistantpb.ChatEvent{
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE},
		}}, nil
	}}
	logic := NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service})
	// 无缓冲且无读取者：发送分支永未就绪，select 必然命中已取消的 ctx.Done。
	client := make(chan *types.AssistantChatEvent)
	err := logic.AssistantChat(&types.AssistantChatReq{Message: "hello"}, client)
	require.ErrorIs(t, err, context.Canceled)
}
