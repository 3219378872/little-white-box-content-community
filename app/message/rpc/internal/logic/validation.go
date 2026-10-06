package logic

const (
	maxMessageContentLength      = 1000
	maxMessageIdempotencyKeySize = 128
	maxNotificationTitleLength   = 100
	maxNotificationContentLength = 500

	messageTypeText  = 1
	messageTypeImage = 2
	messageTypeVideo = 3
	messageTypeAudio = 4
)

// validMessageType 判断私信类型是否为文本、图片、视频或音频之一。
func validMessageType(msgType int32) bool {
	return msgType >= 1 && msgType <= 4
}

// validNotificationType 判断通知类型是否在已定义的 1～5 范围内。
func validNotificationType(notificationType int32) bool {
	return notificationType >= 1 && notificationType <= 5
}

// runeLen 按字符数计算长度，多字节字符计为 1。
func runeLen(value string) int {
	return len([]rune(value))
}
