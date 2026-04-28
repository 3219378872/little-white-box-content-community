package mqs

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"esx/app/message/mq/internal/model"
	"esx/app/message/mq/internal/svc"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeResult struct{ id int64 }

func (r fakeResult) LastInsertId() (int64, error) { return r.id, nil }
func (r fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeNotificationModel struct {
	inserted  []*model.Notification
	insertErr error
}

func (m *fakeNotificationModel) Insert(ctx context.Context, n *model.Notification) (sql.Result, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	m.inserted = append(m.inserted, n)
	return fakeResult{id: 1}, nil
}

type fakeUnreadStore struct{ deleted []int64 }

func (s *fakeUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	s.deleted = append(s.deleted, userID)
	return nil
}

// --- tests ---

func TestMessageConsumer_MalformedJSON_ReturnsSuccess(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`not-json`)}, MsgId: "bad-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, notifications.inserted)
}

func TestMessageConsumer_MissingTargetUserID_ReturnsSuccess(t *testing.T) {
	svcCtx := &svc.ServiceContext{NotificationModel: &fakeNotificationModel{}, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"action_type":1}`)}, MsgId: "msg-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestMessageConsumer_MissingActionType_ReturnsSuccess(t *testing.T) {
	svcCtx := &svc.ServiceContext{NotificationModel: &fakeNotificationModel{}, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9}`)}, MsgId: "msg-2"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestMessageConsumer_UnsupportedActionType_ReturnsSuccess(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":99}`)}, MsgId: "msg-3"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, notifications.inserted)
}

func TestMessageConsumer_LikeNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`)}, MsgId: "msg-4"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, int64(9), notifications.inserted[0].UserId)
	assert.Equal(t, int64(1), notifications.inserted[0].Type)
	assert.Equal(t, "点赞", notifications.inserted[0].Title.String)
	assert.Equal(t, "小白 赞了你的帖子", notifications.inserted[0].Content.String)
	assert.Equal(t, []int64{9}, store.deleted)
}

func TestMessageConsumer_CommentNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":2,"user_id":7,"username":"小黑","target_id":88}`)}, MsgId: "msg-5"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, "评论", notifications.inserted[0].Title.String)
	assert.Equal(t, "小黑 评论了你的帖子", notifications.inserted[0].Content.String)
}

func TestMessageConsumer_FollowNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":3,"user_id":7,"username":"小蓝"}`)}, MsgId: "msg-6"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, "关注", notifications.inserted[0].Title.String)
	assert.Equal(t, "小蓝 关注了你", notifications.inserted[0].Content.String)
}

func TestMessageConsumer_SystemNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":4,"content":"系统维护通知"}`)}, MsgId: "msg-7"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, "系统通知", notifications.inserted[0].Title.String)
	assert.Equal(t, "系统维护通知", notifications.inserted[0].Content.String)
}

func TestMessageConsumer_InsertFails_ReturnsRetry(t *testing.T) {
	notifications := &fakeNotificationModel{insertErr: errors.New("db offline")}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`)}, MsgId: "msg-8"},
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestMessageConsumer_BatchSkipsPermanentAndProcessesRest(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "bad"},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`)}, MsgId: "good"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, notifications.inserted, 1)
}
