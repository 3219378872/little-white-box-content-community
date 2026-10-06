package logic

import (
	"database/sql"
	model2 "esx/app/message/rpc/internal/model"
	pb "esx/kitex_gen/message"
	"esx/pkg/pageutil"
	"strings"
	"time"
)

const (
	defaultPageSize = int32(20)
	maxPageSize     = int32(100)
)

// normalizePage 把页码与页大小规范到默认值与上限之内。
func normalizePage(page, pageSize int32) (int64, int64) {
	return int64(pageutil.ClampPage(page)), int64(pageutil.ClampPageSizeTo(pageSize, defaultPageSize, maxPageSize))
}

// unixMilli 把时间转为毫秒时间戳，零值返回 0。
func unixMilli(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// nullString 把可空字符串转为普通字符串，NULL 视为空串。
func nullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// nullInt64 把可空整数转为普通整数，NULL 视为 0。
func nullInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

// nullableString 去掉首尾空白，空串存为 NULL。
func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

// toNotificationInfo 把通知行转换为响应结构。
func toNotificationInfo(row *model2.Notification) *pb.NotificationInfo {
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

// toMessageInfo 把私信行转换为响应结构；会话 ID 由调用方按查看者视角传入。
func toMessageInfo(row *model2.Message, conversationID int64) *pb.MessageInfo {
	return &pb.MessageInfo{
		Id:             row.Id,
		ConversationId: conversationID,
		SenderId:       row.SenderId,
		ReceiverId:     row.ReceiverId,
		Content:        row.Content,
		MsgType:        int32(row.MsgType),
		Status:         int32(row.Status),
		CreatedAt:      unixMilli(row.CreatedAt),
		MediaId:        nullInt64(row.MediaId),
	}
}

// toConversationInfo 把会话行转换为响应结构。
func toConversationInfo(row *model2.Conversation) *pb.ConversationInfo {
	return &pb.ConversationInfo{
		Id:              row.Id,
		UserId:          row.UserId,
		TargetUserId:    row.TargetUserId,
		LastMessage:     nullString(row.LastMessage),
		LastMessageTime: unixMilli(row.LastMessageTime.Time),
		UnreadCount:     int32(row.UnreadCount),
	}
}
