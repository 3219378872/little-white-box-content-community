package middleware

// SafeAccessLogMiddleware records request metadata only. Headers, query values,
// request bodies and response bodies are deliberately excluded.
type SafeAccessLogMiddleware struct{}

// NewSafeAccessLogMiddleware creates the access log middleware.
func NewSafeAccessLogMiddleware() *SafeAccessLogMiddleware {
	return &SafeAccessLogMiddleware{}
}
