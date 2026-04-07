package errx

import (
	"errors"
	"fmt"
)

// BizError 业务错误
type BizError struct {
	Code    int
	Message string
}

func (e *BizError) Error() string {
	return fmt.Sprintf("code: %d, message: %s", e.Code, e.Message)
}

// BizCode 返回业务错误码，供 result 包通过接口提取
func (e *BizError) BizCode() int {
	return e.Code
}

// New 创建业务错误
func New(code int, message string) error {
	return &BizError{
		Code:    code,
		Message: message,
	}
}

// NewWithCode 使用预定义消息创建业务错误
func NewWithCode(code int) error {
	return &BizError{
		Code:    code,
		Message: GetMsg(code),
	}
}

// Is 判断错误是否为指定错误码
func Is(err error, code int) bool {
	if bizErr, ok := errors.AsType[*BizError](err); ok {
		return bizErr.Code == code
	}
	return false
}

// GetCode 获取错误码
func GetCode(err error) int {
	if bizErr, ok := errors.AsType[*BizError](err); ok {
		return bizErr.Code
	}
	return UnknownError
}

// Wrap 包装错误
func Wrap(err error, code int) error {
	if err == nil {
		return nil
	}
	return &BizError{
		Code:    code,
		Message: err.Error(),
	}
}

// WrapMsg 包装错误并自定义消息
func WrapMsg(err error, message string) error {
	if err == nil {
		return nil
	}
	return &BizError{
		Code:    UnknownError,
		Message: message + ": " + err.Error(),
	}
}
