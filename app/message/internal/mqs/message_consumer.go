package mqs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"esx/app/message/internal/model"
	"esx/app/message/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	NotificationTypeLike    = int64(1)
	NotificationTypeComment = int64(2)
	NotificationTypeFollow  = int64(3)
	NotificationTypeSystem  = int64(4)
)

var ErrPermanentEvent = errors.New("permanent message event error")

func newPermanentEventError(message string) error {
	return fmt.Errorf("%w: %s", ErrPermanentEvent, message)
}

func IsPermanentEventError(err error) bool {
	return errors.Is(err, ErrPermanentEvent)
}

func consumeResultForError(ctx context.Context, msgID string, err error) consumer.ConsumeResult {
	if IsPermanentEventError(err) {
		logx.WithContext(ctx).Errorw("skip permanent message notification event", logx.Field("msg_id", msgID), logx.Field("err", err.Error()))
		return consumer.ConsumeSuccess
	}
	logx.WithContext(ctx).Errorw("consume message notification event failed", logx.Field("msg_id", msgID), logx.Field("err", err.Error()))
	return consumer.ConsumeRetryLater
}

type UserActionEvent struct {
	TargetUserID int64  `json:"target_user_id"`
	ActionType   int64  `json:"action_type"`
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	TargetID     int64  `json:"target_id"`
	TargetType   int64  `json:"target_type"`
	Content      string `json:"content"`
}

type MessageConsumer struct {
	svcCtx *svc.ServiceContext
}

func NewMessageConsumer(svcCtx *svc.ServiceContext) *MessageConsumer {
	return &MessageConsumer{svcCtx: svcCtx}
}

func NewRocketMQConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("create message consumer: %w", err)
	}
	messageConsumer := NewMessageConsumer(svcCtx)
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeMessageBatch(ctx, messageConsumer, msgs...), nil
	}
	if err := c.SubscribeWithTopic(svcCtx.Config.MQ.Topic, svcCtx.Config.MQ.Tag, handler); err != nil {
		return nil, fmt.Errorf("subscribe message topic: %w", err)
	}
	return c, nil
}

func consumeMessageBatch(ctx context.Context, messageConsumer *MessageConsumer, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		if err := messageConsumer.Consume(ctx, msg.Body); err != nil {
			result := consumeResultForError(ctx, msg.MsgId, err)
			if result == consumer.ConsumeRetryLater {
				return result
			}
		}
	}
	return consumer.ConsumeSuccess
}

func (c *MessageConsumer) Consume(ctx context.Context, body []byte) error {
	var event UserActionEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("%w: unmarshal user action event: %v", ErrPermanentEvent, err)
	}
	if event.TargetUserID <= 0 {
		return newPermanentEventError("missing target_user_id")
	}
	if event.ActionType <= 0 {
		return newPermanentEventError("missing action_type")
	}
	title, content := RenderNotificationContent(event)
	if title == "" {
		return newPermanentEventError("unsupported action_type")
	}
	if strings.TrimSpace(content) == "" {
		return newPermanentEventError("empty notification content")
	}
	_, err := c.svcCtx.NotificationModel.Insert(ctx, &model.Notification{
		UserId:     event.TargetUserID,
		Type:       event.ActionType,
		Title:      sql.NullString{String: title, Valid: title != ""},
		Content:    sql.NullString{String: content, Valid: content != ""},
		TargetId:   sql.NullInt64{Int64: event.TargetID, Valid: event.TargetID > 0},
		TargetType: sql.NullInt64{Int64: event.TargetType, Valid: event.TargetType > 0},
		SenderId:   sql.NullInt64{Int64: event.UserID, Valid: event.UserID > 0},
		Status:     0,
	})
	if err != nil {
		logx.WithContext(ctx).Errorw("NotificationModel.Insert failed", logx.Field("err", err.Error()))
		return err
	}
	if c.svcCtx.UnreadStore != nil {
		if err := c.svcCtx.UnreadStore.DeleteUserUnread(ctx, event.TargetUserID); err != nil {
			logx.WithContext(ctx).Errorw("UnreadStore.DeleteUserUnread failed", logx.Field("err", err.Error()))
		}
	}
	return nil
}

func RenderNotificationContent(event UserActionEvent) (string, string) {
	username := strings.TrimSpace(event.Username)
	if username == "" {
		username = "有人"
	}
	switch event.ActionType {
	case NotificationTypeLike:
		return "点赞", fmt.Sprintf("%s 赞了你的帖子", username)
	case NotificationTypeComment:
		return "评论", fmt.Sprintf("%s 评论了你的帖子", username)
	case NotificationTypeFollow:
		return "关注", fmt.Sprintf("%s 关注了你", username)
	case NotificationTypeSystem:
		return "系统通知", strings.TrimSpace(event.Content)
	default:
		return "", ""
	}
}
