package httpxconfig

import (
	"errors"

	"esx/pkg/errx"
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

// ConfigureErrors is retained for existing setup callers; error mapping is explicit.
func ConfigureErrors() {}
