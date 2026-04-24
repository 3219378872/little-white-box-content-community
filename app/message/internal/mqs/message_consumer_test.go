package mqs

import (
	"context"
	"database/sql"
	"testing"

	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"

	"github.com/stretchr/testify/require"
)

type fakeResult struct{ id int64 }

func (r fakeResult) LastInsertId() (int64, error) { return r.id, nil }
func (r fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeNotificationModel struct{ inserted []*model.Notification }

func (m *fakeNotificationModel) Insert(ctx context.Context, data *model.Notification) (sql.Result, error) {
	m.inserted = append(m.inserted, data)
	return fakeResult{id: 1}, nil
}

func (m *fakeNotificationModel) FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*model.Notification, int64, error) {
	return nil, 0, nil
}

func (m *fakeNotificationModel) CountUnread(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

func (m *fakeNotificationModel) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

type fakeUnreadStore struct{ deleted []int64 }

func (s *fakeUnreadStore) GetMessageUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return 0, false, nil
}
func (s *fakeUnreadStore) SetMessageUnread(ctx context.Context, userID int64, count int64) error {
	return nil
}
func (s *fakeUnreadStore) GetNotificationUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return 0, false, nil
}
func (s *fakeUnreadStore) SetNotificationUnread(ctx context.Context, userID int64, count int64) error {
	return nil
}
func (s *fakeUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	s.deleted = append(s.deleted, userID)
	return nil
}

func TestMessageConsumerCreatesLikeNotification(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	consumer := NewMessageConsumer(&svc.ServiceContext{NotificationModel: notifications, UnreadStore: store})

	err := consumer.Consume(context.Background(), []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`))

	require.NoError(t, err)
	require.Len(t, notifications.inserted, 1)
	require.Equal(t, int64(9), notifications.inserted[0].UserId)
	require.Equal(t, int64(1), notifications.inserted[0].Type)
	require.Equal(t, "点赞", notifications.inserted[0].Title.String)
	require.Equal(t, "小白 赞了你的帖子", notifications.inserted[0].Content.String)
	require.Equal(t, []int64{9}, store.deleted)
}

func TestRenderNotificationContentSupportsCommentAndFollow(t *testing.T) {
	commentTitle, commentContent := RenderNotificationContent(UserActionEvent{ActionType: NotificationTypeComment, Username: "小黑"})
	followTitle, followContent := RenderNotificationContent(UserActionEvent{ActionType: NotificationTypeFollow, Username: "小蓝"})

	require.Equal(t, "评论", commentTitle)
	require.Equal(t, "小黑 评论了你的帖子", commentContent)
	require.Equal(t, "关注", followTitle)
	require.Equal(t, "小蓝 关注了你", followContent)
}

func TestMessageConsumerRejectsMalformedPayload(t *testing.T) {
	consumer := NewMessageConsumer(&svc.ServiceContext{})

	err := consumer.Consume(context.Background(), []byte(`not-json`))

	require.Error(t, err)
}
