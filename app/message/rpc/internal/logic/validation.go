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

func validMessageType(msgType int32) bool {
	return msgType >= 1 && msgType <= 4
}

func validNotificationType(notificationType int32) bool {
	return notificationType >= 1 && notificationType <= 5
}

func runeLen(value string) int {
	return len([]rune(value))
}
