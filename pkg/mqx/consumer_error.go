package mqx

import (
	"errors"
	"fmt"
)

// permanentEventError 标记无论重试多少次都无法处理的消息。
type permanentEventError struct {
	reason string
}

// Error 返回带原因的错误文本。
func (e permanentEventError) Error() string {
	return fmt.Sprintf("permanent event: %s", e.reason)
}

// ErrPermanentEvent 包装永久失败：消费者据此确认并丢弃消息，而不是无限重试。
func ErrPermanentEvent(reason string) error {
	return permanentEventError{reason: reason}
}

// IsPermanentEvent 判断错误链中是否有永久失败标记。
func IsPermanentEvent(err error) bool {
	var target permanentEventError
	return errors.As(err, &target)
}
