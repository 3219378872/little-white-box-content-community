package httpxconfig

import (
	"context"
	"errors"
	"net/http"

	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// MapError 将任意错误转换为网关公开的 (HTTP 状态, JSON 信封)。
// HTTPStatus() 是业务码到 HTTP 状态的唯一映射（含 SearchEmpty/SearchTimeout）。
func MapError(err error) (int, any) {
	bizErr, ok := errors.AsType[*errx.BizError](err)
	if !ok {
		bizErr = errx.FromHTTPError(err)
	}
	return bizErr.HTTPStatus(), map[string]any{
		"code":    bizErr.Code,
		"message": bizErr.Message,
	}
}

// ConfigureErrors installs the Gateway's public JSON error contract.
func ConfigureErrors() {
	httpx.SetErrorHandlerCtx(func(_ context.Context, err error) (int, any) {
		return MapError(err)
	})
}

// Unauthorized writes the public error envelope for go-zero JWT failures.
func Unauthorized(w http.ResponseWriter, _ *http.Request, _ error) {
	httpx.WriteJson(w, http.StatusUnauthorized, map[string]any{
		"code":    errx.LoginRequired,
		"message": errx.GetMsg(errx.LoginRequired),
	})
}
