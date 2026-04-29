package logic

import (
	"fmt"
	"strings"
)

const (
	NotificationTypeLike    = int64(1)
	NotificationTypeComment = int64(2)
	NotificationTypeFollow  = int64(3)
	NotificationTypeSystem  = int64(4)
)

type UserActionEvent struct {
	TargetUserID int64
	ActionType   int64
	UserID       int64
	Username     string
	TargetID     int64
	TargetType   int64
	Content      string
}

// RenderNotification returns title and content for a user action event.
// Returns empty strings when the action type is unsupported.
func RenderNotification(event UserActionEvent) (string, string) {
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
