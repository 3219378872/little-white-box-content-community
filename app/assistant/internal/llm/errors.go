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

// ErrorKind 是 provider 错误的归类，决定是否重试以及向用户展示的错误。
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

// ProviderError 是归类后的 provider 错误；HTTP 错误的 Message 只含类别与状态码，不含响应正文。
type ProviderError struct {
	Kind       ErrorKind
	StatusCode int
	Retryable  bool
	RetryAfter time.Duration
	Message    string
	Err        error
}

// Error 返回 Message，缺省时只给出错误类别。
func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	if e.Message != "" {
		return e.Message
	}
	return "provider error: " + string(e.Kind)
}

// Unwrap 暴露底层错误以便 errors.Is 判断超时等情况。
func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ClassifyError 把任意调用错误归类：已归类的原样返回，取消不重试，超时与未知网络错误可重试。
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

// classifyHTTPError 按状态码与响应关键字归类 HTTP 错误；限流、超时、过载与 5xx 可重试。
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

// parseRetryAfter 解析 Retry-After 的秒数或 HTTP 日期，无效或已过期时返回 0。
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
