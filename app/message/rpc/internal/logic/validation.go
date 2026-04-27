package logic

const (
	maxMessageContentLength      = 1000
	maxNotificationTitleLength   = 100
	maxNotificationContentLength = 500
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
