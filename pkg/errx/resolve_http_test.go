package errx

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestFromHTTPError_Nil(t *testing.T) {
	if got := FromHTTPError(nil); got != nil {
		t.Errorf("FromHTTPError(nil) = %v, want nil", got)
	}
}

func TestFromHTTPError_AlreadyBizError(t *testing.T) {
	original := &BizError{Code: UserNotFound, Message: "用户不存在"}
	got := FromHTTPError(original)

	if got != original {
		t.Errorf("FromHTTPError(BizError) returned different pointer")
	}
}

func TestFromHTTPError_MappingErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"field not set", fmt.Errorf("field %q is not set", "username")},
		{"type mismatch", fmt.Errorf("type mismatch for field %q", "loginType")},
		{"option validation", fmt.Errorf(`value "x" for field "type" is not defined in options "[1,2]"`)},
		{"path strconv", fmt.Errorf(`strconv.ParseInt: parsing "not-a-number": invalid syntax`)},
		{"truncated json", fmt.Errorf("unexpected EOF")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromHTTPError(tt.err)
			if got.Code != ParamError {
				t.Errorf("Code = %d, want %d", got.Code, ParamError)
			}
			if got.Message != GetMsg(ParamError) {
				t.Errorf("Message = %q, want generic ParamError", got.Message)
			}
		})
	}
}

func TestFromHTTPError_JSONErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"SyntaxError", &json.SyntaxError{Offset: 1}},
		{"UnmarshalTypeError", &json.UnmarshalTypeError{Field: "id", Type: reflect.TypeFor[int64]()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromHTTPError(tt.err)
			if got.Code != ParamError {
				t.Errorf("Code = %d, want %d", got.Code, ParamError)
			}
			if got.Message != GetMsg(ParamError) {
				t.Errorf("Message = %q, want generic ParamError", got.Message)
			}
		})
	}
}

func TestFromHTTPError_UnknownErrorIsSystemError(t *testing.T) {
	got := FromHTTPError(fmt.Errorf("dial tcp 10.0.0.1:3306: connect refused"))
	if got.Code != SystemError {
		t.Errorf("Code = %d, want %d", got.Code, SystemError)
	}
	if got.Message != GetMsg(SystemError) {
		t.Errorf("Message = %q, want generic SystemError", got.Message)
	}
}

func TestFromHTTPError_WrappedBizError(t *testing.T) {
	inner := &BizError{Code: TokenExpired, Message: "Token已过期"}
	wrapped := fmt.Errorf("parse failed: %w", inner)

	got := FromHTTPError(wrapped)

	if got.Code != TokenExpired {
		t.Errorf("Code = %d, want %d", got.Code, TokenExpired)
	}
	if got.Message != "Token已过期" {
		t.Errorf("Message = %q, want %q", got.Message, "Token已过期")
	}
}
