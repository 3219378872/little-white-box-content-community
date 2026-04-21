package errx

import "errors"

// FromHTTPError converts a non-BizError (from httpx.Parse validation/JSON parsing)
// to a BizError with ParamError code. If err already is a BizError (possibly wrapped),
// it is returned unchanged. nil is returned as-is.
func FromHTTPError(err error) *BizError {
	if err == nil {
		return nil
	}

	if bizErr, ok := errors.AsType[*BizError](err); ok {
		return bizErr
	}

	return &BizError{
		Code:    ParamError,
		Message: err.Error(),
	}
}
