package errx

import (
	"errors"
	"fmt"
	"net/http"
)

// BizError 业务错误
type BizError struct {
	Code    int
	Message string
	cause   error
}

func (e *BizError) Error() string {
	return fmt.Sprintf("code: %d, message: %s", e.Code, e.Message)
}

// Unwrap 返回底层错误，支持 errors.Is / errors.As 穿透
func (e *BizError) Unwrap() error {
	return e.cause
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

// Wrap 包装错误，保留原始错误用于 errors.Is / errors.As 穿透
func Wrap(err error, code int) error {
	if err == nil {
		return nil
	}
	return &BizError{
		Code:    code,
		Message: GetMsg(code),
		cause:   err,
	}
}

// WrapMsg 包装错误并自定义消息，保留原始错误用于 errors.Is / errors.As 穿透
func WrapMsg(err error, message string) error {
	if err == nil {
		return nil
	}
	return &BizError{
		Code:    UnknownError,
		Message: message, // 不再拼接 err.Error()，防止内部信息泄漏
		cause:   err,
	}
}

// HTTPStatus maps business error codes to HTTP status codes.
func (e *BizError) HTTPStatus() int {
	switch {
	case e.Code == SUCCESS:
		return http.StatusOK
	case e.Code == ParamError:
		return http.StatusBadRequest
	case e.Code == NotFound, e.Code == UserNotFound, e.Code == ContentNotFound, e.Code == MediaNotFound:
		return http.StatusNotFound
	case e.Code == LoginRequired, e.Code == TokenExpired, e.Code == TokenInvalid:
		return http.StatusUnauthorized
	case e.Code == PermissionDenied, e.Code == ContentForbidden, e.Code == FavoritesPrivate:
		return http.StatusForbidden
	case e.Code == TooManyReq:
		return http.StatusTooManyRequests
	case e.Code == UserAlreadyExist:
		return http.StatusConflict
	case e.Code == AlreadyLiked, e.Code == AlreadyFavorited, e.Code == NotLikedYet, e.Code == NotFavoritedYet,
		e.Code == CannotLikeSelf, e.Code == CannotFollowSelf:
		return http.StatusBadRequest
	case e.Code == TitleEmpty, e.Code == ContentEmpty, e.Code == ContentTooLong,
		e.Code == FileTooLarge, e.Code == FileTypeNotAllowed, e.Code == MediaMetaMissing:
		return http.StatusBadRequest
	case e.Code == PostAlreadyDeleted:
		return http.StatusGone
	case e.Code == ServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
