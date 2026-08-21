package message

import (
	"context"
	"testing"

	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/message/rpc/messageservice"
	messagepb "esx/app/message/rpc/xiaobaihe/message/pb"
	"esx/pkg/jwtx"

	"google.golang.org/grpc"
)

type fakeMessageService struct {
	messageservice.MessageService
	getConversationsFn func(context.Context, *messageservice.GetConversationsReq, ...grpc.CallOption) (*messageservice.GetConversationsResp, error)
	getMessagesFn      func(context.Context, *messageservice.GetMessagesReq, ...grpc.CallOption) (*messageservice.GetMessagesResp, error)
	sendMessageFn      func(context.Context, *messageservice.SendMessageReq, ...grpc.CallOption) (*messageservice.SendMessageResp, error)
	markReadFn         func(context.Context, *messageservice.MarkReadReq, ...grpc.CallOption) (*messageservice.MarkReadResp, error)
	getUnreadCountFn   func(context.Context, *messageservice.GetUnreadCountReq, ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error)
}

func (f *fakeMessageService) GetConversations(ctx context.Context, in *messageservice.GetConversationsReq, opts ...grpc.CallOption) (*messageservice.GetConversationsResp, error) {
	return f.getConversationsFn(ctx, in, opts...)
}

func (f *fakeMessageService) GetMessages(ctx context.Context, in *messageservice.GetMessagesReq, opts ...grpc.CallOption) (*messageservice.GetMessagesResp, error) {
	return f.getMessagesFn(ctx, in, opts...)
}

func (f *fakeMessageService) SendMessage(ctx context.Context, in *messageservice.SendMessageReq, opts ...grpc.CallOption) (*messageservice.SendMessageResp, error) {
	return f.sendMessageFn(ctx, in, opts...)
}

func (f *fakeMessageService) MarkRead(ctx context.Context, in *messageservice.MarkReadReq, opts ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
	return f.markReadFn(ctx, in, opts...)
}

func (f *fakeMessageService) GetUnreadCount(ctx context.Context, in *messageservice.GetUnreadCountReq, opts ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error) {
	return f.getUnreadCountFn(ctx, in, opts...)
}

func TestGetConversations_UsesJWTUserAndMapsResponse(t *testing.T) {
	type requestContextKey struct{}
	ctxKey := requestContextKey{}
	ctx := context.WithValue(jwtx.WithUserIdContext(context.Background(), 42), ctxKey, "preserved")
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		getConversationsFn: func(gotCtx context.Context, in *messageservice.GetConversationsReq, _ ...grpc.CallOption) (*messageservice.GetConversationsResp, error) {
			if gotCtx.Value(ctxKey) != "preserved" {
				t.Fatal("request context was not propagated")
			}
			if in.UserId != 42 || in.Page != 2 || in.PageSize != 10 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &messageservice.GetConversationsResp{
				Conversations: []*messagepb.ConversationInfo{{
					Id: 1, TargetUserId: 7, TargetUserName: "alice", TargetUserAvatar: "avatar",
					LastMessage: "hello", LastMessageTime: 100, UnreadCount: 2,
				}},
				Total: 1,
			}, nil
		},
	}}

	resp, err := NewGetConversationsLogic(ctx, svcCtx).GetConversations(&types.GetConversationsReq{Page: 2, PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Total != 1 || len(resp.Conversations) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	item := resp.Conversations[0]
	if item.Id != 1 || item.TargetUserId != 7 || item.TargetUserName != "alice" || item.LastMessage != "hello" || item.UnreadCount != 2 {
		t.Fatalf("conversation was not mapped: %+v", item)
	}
}

func TestGetMessages_UsesJWTUserAndMapsResponse(t *testing.T) {
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		getMessagesFn: func(_ context.Context, in *messageservice.GetMessagesReq, _ ...grpc.CallOption) (*messageservice.GetMessagesResp, error) {
			if in.UserId != 42 || in.ConversationId != 9 || in.LastId != 100 || in.PageSize != 20 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &messageservice.GetMessagesResp{
				Messages: []*messagepb.MessageInfo{{
					Id: 101, ConversationId: 9, SenderId: 42, ReceiverId: 7, Content: "hello",
					MsgType: 1, Status: 0, CreatedAt: 1000,
				}},
				HasMore: true,
			}, nil
		},
	}}

	resp, err := NewGetMessagesLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx).GetMessages(&types.GetMessagesReq{
		ConversationId: 9,
		LastId:         100,
		PageSize:       20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.HasMore || len(resp.Messages) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	item := resp.Messages[0]
	if item.Id != 101 || item.ConversationId != 9 || item.SenderId != 42 || item.ReceiverId != 7 || item.Content != "hello" || item.MsgType != 1 || item.CreatedAt != 1000 {
		t.Fatalf("message was not mapped: %+v", item)
	}
}

func TestSendMessage_DerivesSenderFromJWT(t *testing.T) {
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		sendMessageFn: func(_ context.Context, in *messageservice.SendMessageReq, _ ...grpc.CallOption) (*messageservice.SendMessageResp, error) {
			if in.SenderId != 42 || in.ReceiverId != 7 || in.Content != "hello" || in.MsgType != 1 || in.IdempotencyKey != "send-1" {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &messageservice.SendMessageResp{MessageId: 123}, nil
		},
	}}

	resp, err := NewSendMessageLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx).SendMessage(&types.SendMessageReq{
		ReceiverId:     7,
		Content:        "hello",
		MsgType:        1,
		IdempotencyKey: " send-1 ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageId != 123 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestMarkConversationRead_UsesJWTUserAndPropagatesContext(t *testing.T) {
	type requestContextKey struct{}
	ctxKey := requestContextKey{}
	ctx := context.WithValue(jwtx.WithUserIdContext(context.Background(), 42), ctxKey, "preserved")
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		markReadFn: func(gotCtx context.Context, in *messageservice.MarkReadReq, _ ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
			if gotCtx.Value(ctxKey) != "preserved" {
				t.Fatal("request context was not propagated")
			}
			if in.UserId != 42 || in.ConversationId != 9 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &messageservice.MarkReadResp{}, nil
		},
	}}

	resp, err := NewMarkConversationReadLogic(ctx, svcCtx).MarkConversationRead(&types.MarkConversationReadReq{ConversationId: 9})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response")
	}
}

func TestGetUnreadSummary_UsesJWTUserAndMapsResponse(t *testing.T) {
	type requestContextKey struct{}
	ctxKey := requestContextKey{}
	ctx := context.WithValue(jwtx.WithUserIdContext(context.Background(), 42), ctxKey, "preserved")
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		getUnreadCountFn: func(gotCtx context.Context, in *messageservice.GetUnreadCountReq, _ ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error) {
			if gotCtx.Value(ctxKey) != "preserved" {
				t.Fatal("request context was not propagated")
			}
			if in.UserId != 42 {
				t.Fatalf("unexpected rpc request: %+v", in)
			}
			return &messageservice.GetUnreadCountResp{MessageUnread: 3, NotificationUnread: 4}, nil
		},
	}}

	resp, err := NewGetUnreadSummaryLogic(ctx, svcCtx).GetUnreadSummary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageUnread != 3 || resp.NotificationUnread != 4 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestMessageLogics_RejectMissingJWTWithoutRPC(t *testing.T) {
	called := false
	fake := &fakeMessageService{
		getConversationsFn: func(context.Context, *messageservice.GetConversationsReq, ...grpc.CallOption) (*messageservice.GetConversationsResp, error) {
			called = true
			return &messageservice.GetConversationsResp{}, nil
		},
		getMessagesFn: func(context.Context, *messageservice.GetMessagesReq, ...grpc.CallOption) (*messageservice.GetMessagesResp, error) {
			called = true
			return &messageservice.GetMessagesResp{}, nil
		},
		sendMessageFn: func(context.Context, *messageservice.SendMessageReq, ...grpc.CallOption) (*messageservice.SendMessageResp, error) {
			called = true
			return &messageservice.SendMessageResp{}, nil
		},
		markReadFn: func(context.Context, *messageservice.MarkReadReq, ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
			called = true
			return &messageservice.MarkReadResp{}, nil
		},
		getUnreadCountFn: func(context.Context, *messageservice.GetUnreadCountReq, ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error) {
			called = true
			return &messageservice.GetUnreadCountResp{}, nil
		},
	}
	svcCtx := &svc.ServiceContext{MessageService: fake}

	tests := []struct {
		name string
		call func() error
	}{
		{name: "conversations", call: func() error {
			_, err := NewGetConversationsLogic(context.Background(), svcCtx).GetConversations(&types.GetConversationsReq{Page: 1, PageSize: 20})
			return err
		}},
		{name: "messages", call: func() error {
			_, err := NewGetMessagesLogic(context.Background(), svcCtx).GetMessages(&types.GetMessagesReq{ConversationId: 1, PageSize: 20})
			return err
		}},
		{name: "send", call: func() error {
			_, err := NewSendMessageLogic(context.Background(), svcCtx).SendMessage(&types.SendMessageReq{ReceiverId: 1, Content: "hello", MsgType: 1, IdempotencyKey: "send-1"})
			return err
		}},
		{name: "mark read", call: func() error {
			_, err := NewMarkConversationReadLogic(context.Background(), svcCtx).MarkConversationRead(&types.MarkConversationReadReq{ConversationId: 1})
			return err
		}},
		{name: "unread summary", call: func() error {
			_, err := NewGetUnreadSummaryLogic(context.Background(), svcCtx).GetUnreadSummary()
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected authentication context error")
			}
		})
	}
	if called {
		t.Fatal("message rpc must not be called without jwt identity")
	}
}

func TestMessageLogics_RPCError(t *testing.T) {
	fake := &fakeMessageService{
		getConversationsFn: func(context.Context, *messageservice.GetConversationsReq, ...grpc.CallOption) (*messageservice.GetConversationsResp, error) {
			return nil, context.DeadlineExceeded
		},
		getMessagesFn: func(context.Context, *messageservice.GetMessagesReq, ...grpc.CallOption) (*messageservice.GetMessagesResp, error) {
			return nil, context.DeadlineExceeded
		},
		sendMessageFn: func(context.Context, *messageservice.SendMessageReq, ...grpc.CallOption) (*messageservice.SendMessageResp, error) {
			return nil, context.DeadlineExceeded
		},
		markReadFn: func(context.Context, *messageservice.MarkReadReq, ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
			return nil, context.DeadlineExceeded
		},
		getUnreadCountFn: func(context.Context, *messageservice.GetUnreadCountReq, ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error) {
			return nil, context.DeadlineExceeded
		},
	}
	svcCtx := &svc.ServiceContext{MessageService: fake}
	ctx := jwtx.WithUserIdContext(context.Background(), 42)

	tests := []struct {
		name string
		call func() error
	}{
		{name: "conversations", call: func() error {
			_, err := NewGetConversationsLogic(ctx, svcCtx).GetConversations(&types.GetConversationsReq{Page: 1, PageSize: 20})
			return err
		}},
		{name: "messages", call: func() error {
			_, err := NewGetMessagesLogic(ctx, svcCtx).GetMessages(&types.GetMessagesReq{ConversationId: 1, PageSize: 20})
			return err
		}},
		{name: "send", call: func() error {
			_, err := NewSendMessageLogic(ctx, svcCtx).SendMessage(&types.SendMessageReq{ReceiverId: 1, Content: "hello", MsgType: 1, IdempotencyKey: "send-1"})
			return err
		}},
		{name: "mark read", call: func() error {
			_, err := NewMarkConversationReadLogic(ctx, svcCtx).MarkConversationRead(&types.MarkConversationReadReq{ConversationId: 1})
			return err
		}},
		{name: "unread summary", call: func() error {
			_, err := NewGetUnreadSummaryLogic(ctx, svcCtx).GetUnreadSummary()
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("expected rpc error")
			}
		})
	}
}

func TestSendMessage_RejectsMissingIdempotencyKeyWithoutRPC(t *testing.T) {
	called := false
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		sendMessageFn: func(context.Context, *messageservice.SendMessageReq, ...grpc.CallOption) (*messageservice.SendMessageResp, error) {
			called = true
			return &messageservice.SendMessageResp{}, nil
		},
	}}

	_, err := NewSendMessageLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx).SendMessage(&types.SendMessageReq{
		ReceiverId: 7, Content: "hello", MsgType: 1,
	})
	if err == nil {
		t.Fatal("expected idempotency key validation error")
	}
	if called {
		t.Fatal("message rpc must not be called without an idempotency key")
	}
}

func TestMarkConversationRead_RejectsInvalidConversationWithoutRPC(t *testing.T) {
	called := false
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		markReadFn: func(context.Context, *messageservice.MarkReadReq, ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
			called = true
			return &messageservice.MarkReadResp{}, nil
		},
	}}
	logic := NewMarkConversationReadLogic(jwtx.WithUserIdContext(context.Background(), 42), svcCtx)

	for _, req := range []*types.MarkConversationReadReq{nil, {}, {ConversationId: -1}} {
		if _, err := logic.MarkConversationRead(req); err == nil {
			t.Fatalf("expected validation error for request %+v", req)
		}
	}
	if called {
		t.Fatal("message rpc must not be called for an invalid conversation")
	}
}

func TestMessageReadLogics_RejectNilRPCResponses(t *testing.T) {
	svcCtx := &svc.ServiceContext{MessageService: &fakeMessageService{
		markReadFn: func(context.Context, *messageservice.MarkReadReq, ...grpc.CallOption) (*messageservice.MarkReadResp, error) {
			return nil, nil
		},
		getUnreadCountFn: func(context.Context, *messageservice.GetUnreadCountReq, ...grpc.CallOption) (*messageservice.GetUnreadCountResp, error) {
			return nil, nil
		},
	}}
	ctx := jwtx.WithUserIdContext(context.Background(), 42)

	if _, err := NewMarkConversationReadLogic(ctx, svcCtx).MarkConversationRead(&types.MarkConversationReadReq{ConversationId: 1}); err == nil {
		t.Fatal("expected nil MarkRead response error")
	}
	if _, err := NewGetUnreadSummaryLogic(ctx, svcCtx).GetUnreadSummary(); err == nil {
		t.Fatal("expected nil GetUnreadCount response error")
	}
}
