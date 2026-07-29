package httpxconfig

import (
	"context"
	"errors"
	"net/http"

	"errx"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ConfigureErrors installs the Gateway's public JSON error contract.
func ConfigureErrors() {
	httpx.SetErrorHandlerCtx(func(_ context.Context, err error) (int, any) {
		bizErr, ok := errors.AsType[*errx.BizError](err)
		if !ok {
			bizErr = errx.FromHTTPError(err)
		}
		status := bizErr.HTTPStatus()
		switch bizErr.Code {
		case errx.SearchEmpty:
			status = http.StatusBadRequest
		case errx.SearchTimeout:
			status = http.StatusGatewayTimeout
		}
		return status, map[string]any{
			"code":    bizErr.Code,
			"message": bizErr.Message,
		}
	})
}

// Unauthorized writes the public error envelope for go-zero JWT failures.
func Unauthorized(w http.ResponseWriter, _ *http.Request, _ error) {
	httpx.WriteJson(w, http.StatusUnauthorized, map[string]any{
		"code":    errx.LoginRequired,
		"message": errx.GetMsg(errx.LoginRequired),
	})
}
