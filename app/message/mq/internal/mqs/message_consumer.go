package mqs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"esx/app/message/mq/internal/model"
	"esx/app/message/mq/internal/svc"
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

type userActionEvent struct {
	TargetUserID int64  `json:"target_user_id"`
	ActionType   int64  `json:"action_type"`
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	TargetID     int64  `json:"target_id"`
	TargetType   int64  `json:"target_type"`
	Content      string `json:"content"`
}

func NewMessageConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("message-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeNotificationBatch(ctx, svcCtx, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicMessagePush, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("message-consumer: subscribe %s: %w", mqx.TopicMessagePush, err)
	}
	return c, nil
}

func consumeNotificationBatch(ctx context.Context, svcCtx *svc.ServiceContext, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event userActionEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("message-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.TargetUserID <= 0 {
			logx.WithContext(ctx).Errorw("message-consumer: missing target_user_id",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		if event.ActionType <= 0 {
			logx.WithContext(ctx).Errorw("message-consumer: missing action_type",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		title, content := renderNotificationContent(event)
		if title == "" {
			logx.WithContext(ctx).Errorw("message-consumer: unsupported action_type",
				logx.Field("msg_id", msg.MsgId), logx.Field("action_type", event.ActionType))
			continue
		}
		if strings.TrimSpace(content) == "" {
			logx.WithContext(ctx).Errorw("message-consumer: empty notification content",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		_, err := svcCtx.NotificationModel.Insert(ctx, &model.Notification{
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
			logx.WithContext(ctx).Errorw("message-consumer: insert notification failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		if svcCtx.UnreadStore != nil {
			if err := svcCtx.UnreadStore.DeleteUserUnread(ctx, event.TargetUserID); err != nil {
				logx.WithContext(ctx).Errorw("message-consumer: delete unread cache failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("target_user_id", event.TargetUserID),
					logx.Field("err", err.Error()))
			}
		}
	}
	return consumer.ConsumeSuccess
}

func renderNotificationContent(event userActionEvent) (string, string) {
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
