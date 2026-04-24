package logic

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"errx"
	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"
	"esx/app/message/xiaobaihe/message/pb"

	"github.com/stretchr/testify/require"
)

type fakeResult struct{ id int64 }

func (r fakeResult) LastInsertId() (int64, error) { return r.id, nil }
func (r fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeNotificationModel struct {
	inserted []*model.Notification
	list     []*model.Notification
	total    int64
	unread   int64
	marked   int64
}

func (m *fakeNotificationModel) Insert(ctx context.Context, data *model.Notification) (sql.Result, error) {
	data.Id = int64(len(m.inserted) + 100)
	data.CreatedAt = time.Unix(10, 0)
	m.inserted = append(m.inserted, data)
	return fakeResult{id: data.Id}, nil
}

func (m *fakeNotificationModel) FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*model.Notification, int64, error) {
	return m.list, m.total, nil
}

func (m *fakeNotificationModel) CountUnread(ctx context.Context, userID int64) (int64, error) {
	return m.unread, nil
}

func (m *fakeNotificationModel) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	m.marked++
	return 2, nil
}

type fakeMessageModel struct {
	inserted []*model.Message
	list     []*model.Message
	hasMore  bool
	unread   int64
	marked   int64
}

func (m *fakeMessageModel) Insert(ctx context.Context, data *model.Message) (sql.Result, error) {
	data.Id = int64(len(m.inserted) + 200)
	data.CreatedAt = time.Unix(20, 0)
	m.inserted = append(m.inserted, data)
	return fakeResult{id: data.Id}, nil
}

func (m *fakeMessageModel) FindByConversation(ctx context.Context, conversationID int64, lastID int64, limit int64) ([]*model.Message, bool, error) {
	return m.list, m.hasMore, nil
}

func (m *fakeMessageModel) CountUnreadByUser(ctx context.Context, userID int64) (int64, error) {
	return m.unread, nil
}

func (m *fakeMessageModel) MarkConversationRead(ctx context.Context, userID int64, conversationID int64) (int64, error) {
	m.marked++
	return 3, nil
}

type fakeConversationModel struct {
	createdSender   int64
	createdReceiver int64
	list            []*model.Conversation
	total           int64
}

func (m *fakeConversationModel) UpsertPairForMessage(ctx context.Context, senderID int64, receiverID int64, content string) (int64, int64, error) {
	m.createdSender = senderID
	m.createdReceiver = receiverID
	return 11, 12, nil
}

func (m *fakeConversationModel) FindByUser(ctx context.Context, userID int64, page int64, pageSize int64) ([]*model.Conversation, int64, error) {
	return m.list, m.total, nil
}

type fakeUnreadStore struct {
	messageValue      int64
	notificationValue int64
	hitMessage        bool
	hitNotification   bool
	setMessage        int64
	setNotification   int64
	deleted           []int64
}

func (s *fakeUnreadStore) GetMessageUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return s.messageValue, s.hitMessage, nil
}

func (s *fakeUnreadStore) SetMessageUnread(ctx context.Context, userID int64, count int64) error {
	s.setMessage = count
	return nil
}

func (s *fakeUnreadStore) GetNotificationUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return s.notificationValue, s.hitNotification, nil
}

func (s *fakeUnreadStore) SetNotificationUnread(ctx context.Context, userID int64, count int64) error {
	s.setNotification = count
	return nil
}

func (s *fakeUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	s.deleted = append(s.deleted, userID)
	return nil
}

func TestSendNotificationCreatesUnreadNotification(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	ctx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	resp, err := NewSendNotificationLogic(context.Background(), ctx).SendNotification(&pb.SendNotificationReq{
		UserId: 7, Type: 4, Title: "公告", Content: "系统维护", TargetId: 9,
	})

	require.NoError(t, err)
	require.Equal(t, int64(100), resp.NotificationId)
	require.Len(t, notifications.inserted, 1)
	require.Equal(t, int64(7), notifications.inserted[0].UserId)
	require.Equal(t, int64(4), notifications.inserted[0].Type)
	require.Equal(t, int64(0), notifications.inserted[0].Status)
	require.Equal(t, []int64{7}, store.deleted)
}

func TestSendNotificationRejectsInvalidRequest(t *testing.T) {
	_, err := NewSendNotificationLogic(context.Background(), &svc.ServiceContext{}).SendNotification(&pb.SendNotificationReq{UserId: 0, Type: 4})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}

func TestGetNotificationsReturnsPagedItems(t *testing.T) {
	createdAt := time.UnixMilli(12345)
	notifications := &fakeNotificationModel{total: 1, list: []*model.Notification{{
		Id: 1, UserId: 7, Type: 1, Title: sql.NullString{String: "点赞", Valid: true}, Content: sql.NullString{String: "小白赞了你", Valid: true}, TargetId: sql.NullInt64{Int64: 99, Valid: true}, Status: 0, CreatedAt: createdAt,
	}}}
	ctx := &svc.ServiceContext{NotificationModel: notifications}

	resp, err := NewGetNotificationsLogic(context.Background(), ctx).GetNotifications(&pb.GetNotificationsReq{UserId: 7, Type: 0, Page: 1, PageSize: 20})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Notifications, 1)
	require.Equal(t, "点赞", resp.Notifications[0].Title)
	require.Equal(t, int64(12345), resp.Notifications[0].CreatedAt)
}

func TestGetUnreadCountUsesRedisHit(t *testing.T) {
	ctx := &svc.ServiceContext{UnreadStore: &fakeUnreadStore{hitMessage: true, messageValue: 3, hitNotification: true, notificationValue: 5}}

	resp, err := NewGetUnreadCountLogic(context.Background(), ctx).GetUnreadCount(&pb.GetUnreadCountReq{UserId: 7})

	require.NoError(t, err)
	require.Equal(t, int32(3), resp.MessageUnread)
	require.Equal(t, int32(5), resp.NotificationUnread)
}

func TestGetUnreadCountFallsBackToDatabaseAndCaches(t *testing.T) {
	messages := &fakeMessageModel{unread: 4}
	notifications := &fakeNotificationModel{unread: 6}
	store := &fakeUnreadStore{}
	ctx := &svc.ServiceContext{MessageModel: messages, NotificationModel: notifications, UnreadStore: store}

	resp, err := NewGetUnreadCountLogic(context.Background(), ctx).GetUnreadCount(&pb.GetUnreadCountReq{UserId: 7})

	require.NoError(t, err)
	require.Equal(t, int32(4), resp.MessageUnread)
	require.Equal(t, int32(6), resp.NotificationUnread)
	require.Equal(t, int64(4), store.setMessage)
	require.Equal(t, int64(6), store.setNotification)
}

func TestMarkReadMarksMessagesAndNotificationsThenInvalidatesCache(t *testing.T) {
	messages := &fakeMessageModel{}
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	ctx := &svc.ServiceContext{MessageModel: messages, NotificationModel: notifications, UnreadStore: store}

	_, err := NewMarkReadLogic(context.Background(), ctx).MarkRead(&pb.MarkReadReq{UserId: 7, ConversationId: 11})

	require.NoError(t, err)
	require.Equal(t, int64(1), messages.marked)
	require.Equal(t, int64(1), notifications.marked)
	require.Equal(t, []int64{7}, store.deleted)
}

func TestSendMessageCreatesConversationAndMessage(t *testing.T) {
	conversations := &fakeConversationModel{}
	messages := &fakeMessageModel{}
	store := &fakeUnreadStore{}
	ctx := &svc.ServiceContext{ConversationModel: conversations, MessageModel: messages, UnreadStore: store}

	resp, err := NewSendMessageLogic(context.Background(), ctx).SendMessage(&pb.SendMessageReq{SenderId: 1, ReceiverId: 2, Content: "hello", MsgType: 1})

	require.NoError(t, err)
	require.Equal(t, int64(200), resp.MessageId)
	require.Equal(t, int64(1), conversations.createdSender)
	require.Equal(t, int64(2), conversations.createdReceiver)
	require.Len(t, messages.inserted, 1)
	require.Equal(t, int64(12), messages.inserted[0].ConversationId)
	require.Equal(t, []int64{2}, store.deleted)
}

func TestGetConversationsReturnsPagedItems(t *testing.T) {
	last := time.UnixMilli(56789)
	conversations := &fakeConversationModel{total: 1, list: []*model.Conversation{{
		Id: 12, UserId: 7, TargetUserId: 8, LastMessage: sql.NullString{String: "hi", Valid: true}, LastMessageTime: sql.NullTime{Time: last, Valid: true}, UnreadCount: 2,
	}}}
	ctx := &svc.ServiceContext{ConversationModel: conversations}

	resp, err := NewGetConversationsLogic(context.Background(), ctx).GetConversations(&pb.GetConversationsReq{UserId: 7, Page: 1, PageSize: 10})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Conversations, 1)
	require.Equal(t, int64(8), resp.Conversations[0].TargetUserId)
	require.Equal(t, int32(2), resp.Conversations[0].UnreadCount)
}

func TestGetMessagesReturnsCursorItems(t *testing.T) {
	createdAt := time.UnixMilli(45678)
	messages := &fakeMessageModel{hasMore: true, list: []*model.Message{{
		Id: 1, ConversationId: 12, SenderId: 1, ReceiverId: 2, Content: "hi", MsgType: 1, Status: 0, CreatedAt: createdAt,
	}}}
	ctx := &svc.ServiceContext{MessageModel: messages}

	resp, err := NewGetMessagesLogic(context.Background(), ctx).GetMessages(&pb.GetMessagesReq{ConversationId: 12, PageSize: 20})

	require.NoError(t, err)
	require.True(t, resp.HasMore)
	require.Len(t, resp.Messages, 1)
	require.Equal(t, "hi", resp.Messages[0].Content)
	require.Equal(t, int64(45678), resp.Messages[0].CreatedAt)
}

func TestSendMessageRejectsInvalidRequest(t *testing.T) {
	_, err := NewSendMessageLogic(context.Background(), &svc.ServiceContext{}).SendMessage(&pb.SendMessageReq{SenderId: 1, ReceiverId: 1, Content: "hello", MsgType: 1})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}

func TestGetConversationsRejectsInvalidRequest(t *testing.T) {
	_, err := NewGetConversationsLogic(context.Background(), &svc.ServiceContext{}).GetConversations(&pb.GetConversationsReq{UserId: 0})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}

func TestGetMessagesRejectsInvalidRequest(t *testing.T) {
	_, err := NewGetMessagesLogic(context.Background(), &svc.ServiceContext{}).GetMessages(&pb.GetMessagesReq{ConversationId: 0})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}

func TestGetNotificationsRejectsInvalidRequest(t *testing.T) {
	_, err := NewGetNotificationsLogic(context.Background(), &svc.ServiceContext{}).GetNotifications(&pb.GetNotificationsReq{UserId: 0})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}

func TestGetUnreadCountRejectsInvalidRequest(t *testing.T) {
	_, err := NewGetUnreadCountLogic(context.Background(), &svc.ServiceContext{}).GetUnreadCount(&pb.GetUnreadCountReq{UserId: 0})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}

func TestMarkReadRejectsInvalidRequest(t *testing.T) {
	_, err := NewMarkReadLogic(context.Background(), &svc.ServiceContext{}).MarkRead(&pb.MarkReadReq{UserId: 0})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.ParamError))
}
