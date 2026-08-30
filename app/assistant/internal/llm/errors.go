package llm

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ErrorKind string

const (
	ErrorAuth            ErrorKind = "auth"
	ErrorInvalidRequest  ErrorKind = "invalid_request"
	ErrorContextOverflow ErrorKind = "context_overflow"
	ErrorRateLimit       ErrorKind = "rate_limit"
	ErrorTimeout         ErrorKind = "timeout"
	ErrorOverloaded      ErrorKind = "overloaded"
	ErrorServer          ErrorKind = "server_error"
	ErrorContentPolicy   ErrorKind = "content_policy"
	ErrorUnknown         ErrorKind = "unknown"
)

type ProviderError struct {
	Kind       ErrorKind
	StatusCode int
	Retryable  bool
	RetryAfter time.Duration
	Message    string
	Err        error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	if e.Message != "" {
		return e.Message
	}
	return "provider error: " + string(e.Kind)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func ClassifyError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	var classified *ProviderError
	if errors.As(err, &classified) {
		return classified
	}
	if errors.Is(err, context.Canceled) {
		return &ProviderError{Kind: ErrorUnknown, Retryable: false, Message: err.Error(), Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &ProviderError{Kind: ErrorTimeout, Retryable: true, Message: "assistant LLM request timed out", Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &ProviderError{Kind: ErrorTimeout, Retryable: true, Message: "assistant LLM request timed out", Err: err}
	}
	return &ProviderError{Kind: ErrorUnknown, Retryable: true, Message: err.Error(), Err: err}
}

func classifyHTTPError(status int, headers http.Header, raw []byte) *ProviderError {
	text := strings.ToLower(string(raw))
	kind := ErrorUnknown
	retryable := false
	switch {
	case status == http.StatusUnauthorized:
		kind = ErrorAuth
	case status == http.StatusTooManyRequests:
		kind, retryable = ErrorRateLimit, true
	case status == http.StatusRequestTimeout:
		kind, retryable = ErrorTimeout, true
	case status == http.StatusServiceUnavailable || status == 529:
		kind, retryable = ErrorOverloaded, true
	case status >= 500:
		kind, retryable = ErrorServer, true
	case strings.Contains(text, "content_policy") || strings.Contains(text, "content filter") ||
		strings.Contains(text, "safety policy"):
		kind = ErrorContentPolicy
	case strings.Contains(text, "context_length") || strings.Contains(text, "context window") ||
		strings.Contains(text, "maximum context") || strings.Contains(text, "too many tokens"):
		kind = ErrorContextOverflow
	case status == http.StatusForbidden:
		kind = ErrorAuth
	case status >= 400 && status < 500:
		kind = ErrorInvalidRequest
	}
	return &ProviderError{
		Kind: kind, StatusCode: status, Retryable: retryable,
		RetryAfter: parseRetryAfter(headers.Get("Retry-After"), time.Now()),
		Message:    "assistant LLM request failed: kind=" + string(kind) + " status=" + strconv.Itoa(status),
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if seconds < 0 {
			return 0
		}
		return time.Duration(seconds * float64(time.Second))
	}
	when, err := http.ParseTime(value)
	if err != nil || when.Before(now) {
		return 0
	}
	return when.Sub(now)
}
