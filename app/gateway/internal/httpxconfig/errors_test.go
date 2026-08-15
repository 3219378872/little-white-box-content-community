package httpxconfig

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"errx"
)

func TestMapError_BizErrorPassthrough(t *testing.T) {
	status, body := MapError(errx.NewWithCode(errx.PasswordError))
	if status != http.StatusUnauthorized {
		t.Fatalf("PasswordError status = %d, want 401", status)
	}
	envelope, ok := body.(map[string]any)
	if !ok || envelope["code"] != errx.PasswordError {
		t.Fatalf("unexpected envelope: %+v", body)
	}
}

func TestMapError_AuthAndVerifyCodes(t *testing.T) {
	cases := []struct {
		code   int
		status int
	}{
		{errx.PasswordError, http.StatusUnauthorized},
		{errx.VerifyCodeError, http.StatusBadRequest},
		{errx.VerifyCodeExpired, http.StatusBadRequest},
		{errx.SearchEmpty, http.StatusBadRequest},
		{errx.SearchTimeout, http.StatusGatewayTimeout},
		{errx.LoginRequired, http.StatusUnauthorized},
		{errx.IdempotencyConflict, http.StatusConflict},
		{errx.ContentNotFound, http.StatusNotFound},
	}
	for _, tc := range cases {
		status, _ := MapError(errx.NewWithCode(tc.code))
		if status != tc.status {
			t.Errorf("code %d: status = %d, want %d", tc.code, status, tc.status)
		}
	}
}

func TestMapError_WrappedBizError(t *testing.T) {
	inner := errx.NewWithCode(errx.VerifyCodeExpired)
	wrapped := fmt.Errorf("redis: %w", inner)
	status, body := MapError(wrapped)
	if status != http.StatusBadRequest {
		t.Fatalf("wrapped VerifyCodeExpired status = %d, want 400", status)
	}
	envelope, ok := body.(map[string]any)
	if !ok || envelope["code"] != errx.VerifyCodeExpired {
		t.Fatalf("unexpected envelope for wrapped error: %+v", body)
	}
}

func TestMapError_UnknownErrorMapsToSystemError(t *testing.T) {
	status, body := MapError(errors.New("some plain error"))
	if status != http.StatusInternalServerError {
		t.Fatalf("plain error status = %d, want 500", status)
	}
	envelope, ok := body.(map[string]any)
	if !ok || envelope["code"] != errx.SystemError {
		t.Fatalf("unexpected envelope: %+v", body)
	}
	if envelope["message"] != errx.GetMsg(errx.SystemError) {
		t.Fatalf("unexpected message: %+v", envelope)
	}
}
