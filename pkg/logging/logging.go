// Package logging provides contextual structured logging backed by slog.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// LogField is a structured log attribute.
type LogField = slog.Attr

// Logger is the request-scoped logging surface used by handlers and logic.
type Logger interface {
	Errorw(string, ...LogField)
	Infow(string, ...LogField)
	Errorf(string, ...any)
	Error(...any)
}

// contextualLogger attaches the request trace ID to every entry.
type contextualLogger struct{ ctx context.Context }

var mu sync.RWMutex
var output io.Writer = os.Stderr
var logLevel slog.LevelVar
var logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: &logLevel}))

// Field builds a structured attribute.
func Field(key string, value any) LogField { return slog.Any(key, value) }

// WithContext returns a logger bound to ctx so entries carry its trace ID.
func WithContext(ctx context.Context) Logger { return contextualLogger{ctx: ctx} }

// write appends the trace ID when present and emits through the current writer.
func (l contextualLogger) write(level slog.Level, msg string, fields ...LogField) {
	mu.RLock()
	target := logger
	mu.RUnlock()
	if id := TraceID(l.ctx); id != "" {
		fields = append(fields, Field("trace_id", id))
	}
	target.LogAttrs(l.ctx, level, msg, fields...)
}

// Errorw logs an error with structured fields.
func (l contextualLogger) Errorw(msg string, fields ...LogField) {
	l.write(slog.LevelError, msg, fields...)
}

// Infow logs an informational entry with structured fields.
func (l contextualLogger) Infow(msg string, fields ...LogField) {
	l.write(slog.LevelInfo, msg, fields...)
}

// Errorf, Error and the package-level Errorw/Error format their arguments into one error
// entry; the package-level variants have no request context.
func (l contextualLogger) Errorf(format string, args ...any) { l.Errorw(fmt.Sprintf(format, args...)) }
func (l contextualLogger) Error(args ...any)                 { l.Errorw(fmt.Sprint(args...)) }
func Errorw(msg string, fields ...LogField)                  { WithContext(context.Background()).Errorw(msg, fields...) }
func Error(args ...any)                                      { WithContext(context.Background()).Error(args...) }

// Must panics on startup errors that leave the process unusable.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

// NewWriter is kept for call sites that wrap an output writer; it returns w unchanged.
func NewWriter(w io.Writer) io.Writer { return w }

// SetWriter redirects all subsequent log output (tests capture logs this way).
func SetWriter(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	output = w
	logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: &logLevel}))
}

// Reset restores stderr output and returns the writer that was active before.
func Reset() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	previous := output
	output = os.Stderr
	logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: &logLevel}))
	return previous
}

// Disable discards all log output.
func Disable() { SetWriter(io.Discard) }

// traceIDKey is the private context key for the request trace ID.
type traceIDKey struct{}

// WithTraceID stores the request trace ID that later log entries attach.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceID returns the request trace ID, or "" when none was set.
func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

// Config retains the deployment's structured console logger settings.
type Config struct {
	Level       string `json:",default=info,options=debug|info|warn|error"`
	Mode        string `json:",default=console,options=console"`
	Encoding    string `json:",default=json,options=json"`
	ServiceName string
}

// Configure applies the configured minimum level; an unknown level name is a startup error.
func Configure(c Config) error {
	level := slog.LevelInfo
	if c.Level != "" {
		if err := level.UnmarshalText([]byte(c.Level)); err != nil {
			return fmt.Errorf("invalid log level")
		}
	}
	logLevel.Set(level)
	return nil
}
