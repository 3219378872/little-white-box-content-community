package errx

import (
	"net/http"
	"testing"
)

// TestHTTPStatus_MapsEveryBusinessCode 防止业务码落入默认 500：
// 客户端可触发的条件（认证/验证码/参数/空搜索）必须映射为明确的 4xx。
func TestHTTPStatus_MapsEveryBusinessCode(t *testing.T) {
	expected := map[int]int{
		SUCCESS:                http.StatusOK,
		UnknownError:           http.StatusInternalServerError,
		SystemError:            http.StatusInternalServerError,
		ParamError:             http.StatusBadRequest,
		NotFound:               http.StatusNotFound,
		TooManyReq:             http.StatusTooManyRequests,
		ServiceUnavailable:     http.StatusServiceUnavailable,
		UserNotFound:           http.StatusNotFound,
		UserAlreadyExist:       http.StatusConflict,
		PasswordError:          http.StatusUnauthorized,
		TokenExpired:           http.StatusUnauthorized,
		TokenInvalid:           http.StatusUnauthorized,
		LoginRequired:          http.StatusUnauthorized,
		PermissionDenied:       http.StatusForbidden,
		VerifyCodeError:        http.StatusBadRequest,
		VerifyCodeExpired:      http.StatusBadRequest,
		ContentNotFound:        http.StatusNotFound,
		ContentForbidden:       http.StatusForbidden,
		ContentTooLong:         http.StatusBadRequest,
		ContentEmpty:           http.StatusBadRequest,
		TitleEmpty:             http.StatusBadRequest,
		PostAlreadyDeleted:     http.StatusGone,
		ContentVersionConflict: http.StatusConflict,
		IdempotencyConflict:    http.StatusConflict,
		AlreadyLiked:           http.StatusBadRequest,
		AlreadyFavorited:       http.StatusBadRequest,
		NotLikedYet:            http.StatusBadRequest,
		NotFavoritedYet:        http.StatusBadRequest,
		CannotLikeSelf:         http.StatusBadRequest,
		CannotFollowSelf:       http.StatusBadRequest,
		FavoritesPrivate:       http.StatusForbidden,
		FileTooLarge:           http.StatusBadRequest,
		FileTypeNotAllowed:     http.StatusBadRequest,
		UploadFailed:           http.StatusInternalServerError,
		MediaNotFound:          http.StatusNotFound,
		MediaMetaMissing:       http.StatusBadRequest,
		MediaProcessFailed:     http.StatusInternalServerError,
		SearchEmpty:            http.StatusBadRequest,
		SearchTimeout:          http.StatusGatewayTimeout,
	}
	for code, want := range expected {
		got := (&BizError{Code: code}).HTTPStatus()
		if got != want {
			t.Errorf("code %d: HTTPStatus() = %d, want %d", code, got, want)
		}
	}
}
