package errx

import (
	"encoding/json"
	"errors"
	"strings"
)

// FromHTTPError converts a non-BizError (from httpx.Parse validation/JSON parsing)
// to a public BizError. Parse/validation failures become ParamError with the
// generic message. Unknown errors become SystemError. Neither path returns
// the raw err.Error() (CORE-054).
func FromHTTPError(err error) *BizError {
	if err == nil {
		return nil
	}

	if bizErr, ok := errors.AsType[*BizError](err); ok {
		return bizErr
	}

	if isHTTPParseError(err) {
		return &BizError{Code: ParamError, Message: GetMsg(ParamError)}
	}
	return &BizError{Code: SystemError, Message: GetMsg(SystemError)}
}

func isHTTPParseError(err error) bool {
	if _, ok := errors.AsType[*json.SyntaxError](err); ok {
		return true
	}
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "is not set") ||
		strings.Contains(msg, "type mismatch") ||
		strings.Contains(msg, "is not defined in options") ||
		strings.Contains(msg, "unmarshal") ||
		strings.Contains(msg, "invalid character") ||
		strings.Contains(msg, "strconv") ||
		strings.Contains(msg, "invalid syntax") ||
		strings.Contains(msg, "unexpected end") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "request body") ||
		strings.Contains(msg, "not json") ||
		strings.Contains(msg, "multipart") ||
		strings.Contains(msg, "parse")
}
