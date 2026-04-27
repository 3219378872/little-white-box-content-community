package logic

import (
	"database/sql"
	"strings"
	"time"

	"esx/app/message/internal/model"
	"esx/app/message/xiaobaihe/message/pb"
)

const (
	defaultPage     = int32(1)
	defaultPageSize = int32(20)
	maxPageSize     = int32(100)
)

func normalizePage(page, pageSize int32) (int64, int64) {
	if page < 1 {
		page = defaultPage
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return int64(page), int64(pageSize)
}

func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func toNotificationInfo(row *model.Notification) *pb.NotificationInfo {
	return &pb.NotificationInfo{
		Id:        row.Id,
		UserId:    row.UserId,
		Type:      int32(row.Type),
		Title:     nullString(row.Title),
		Content:   nullString(row.Content),
		TargetId:  nullInt64(row.TargetId),
		Status:    int32(row.Status),
		CreatedAt: unixMilli(row.CreatedAt),
	}
}

func toMessageInfo(row *model.Message) *pb.MessageInfo {
	return &pb.MessageInfo{
		Id:             row.Id,
		ConversationId: row.ConversationId,
		SenderId:       row.SenderId,
		ReceiverId:     row.ReceiverId,
		Content:        row.Content,
		MsgType:        int32(row.MsgType),
		Status:         int32(row.Status),
		CreatedAt:      unixMilli(row.CreatedAt),
	}
}

func toConversationInfo(row *model.Conversation) *pb.ConversationInfo {
	return &pb.ConversationInfo{
		Id:              row.Id,
		UserId:          row.UserId,
		TargetUserId:    row.TargetUserId,
		LastMessage:     nullString(row.LastMessage),
		LastMessageTime: unixMilli(row.LastMessageTime.Time),
		UnreadCount:     int32(row.UnreadCount),
	}
}
