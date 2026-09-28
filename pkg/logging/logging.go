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

type LogField = slog.Attr
type Logger interface {
	Errorw(string, ...LogField)
	Infow(string, ...LogField)
	Errorf(string, ...any)
	Error(...any)
}
type contextualLogger struct{ ctx context.Context }

var mu sync.RWMutex
var output io.Writer = os.Stderr
var logLevel slog.LevelVar
var logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: &logLevel}))

func Field(key string, value any) LogField   { return slog.Any(key, value) }
func WithContext(ctx context.Context) Logger { return contextualLogger{ctx: ctx} }
func (l contextualLogger) write(level slog.Level, msg string, fields ...LogField) {
	mu.RLock()
	target := logger
	mu.RUnlock()
	if id := TraceID(l.ctx); id != "" {
		fields = append(fields, Field("trace_id", id))
	}
	target.LogAttrs(l.ctx, level, msg, fields...)
}
func (l contextualLogger) Errorw(msg string, fields ...LogField) {
	l.write(slog.LevelError, msg, fields...)
}
func (l contextualLogger) Infow(msg string, fields ...LogField) {
	l.write(slog.LevelInfo, msg, fields...)
}
func (l contextualLogger) Errorf(format string, args ...any) { l.Errorw(fmt.Sprintf(format, args...)) }
func (l contextualLogger) Error(args ...any)                 { l.Errorw(fmt.Sprint(args...)) }
func Errorw(msg string, fields ...LogField)                  { WithContext(context.Background()).Errorw(msg, fields...) }
func Error(args ...any)                                      { WithContext(context.Background()).Error(args...) }
func Must(err error) {
	if err != nil {
		panic(err)
	}
}
func NewWriter(w io.Writer) io.Writer { return w }
func SetWriter(w io.Writer) {
	mu.Lock()
	defer mu.Unlock()
	output = w
	logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: &logLevel}))
}
func Reset() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	previous := output
	output = os.Stderr
	logger = slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: &logLevel}))
	return previous
}
func Disable() { SetWriter(io.Discard) }

type traceIDKey struct{}

func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}
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
