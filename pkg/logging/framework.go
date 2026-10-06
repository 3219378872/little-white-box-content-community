package logging

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/kitex/pkg/klog"
)

// frameworkLogger 把 Kitex / Hertz 的框架日志转发到统一的 JSON 日志，并按框架独立控制级别。
type frameworkLogger struct {
	component string
	level     atomic.Int32
}

// emit 只记录组件、级别与调用位置，不输出框架原始消息，避免框架日志泄露请求内容；
// 级别 4 记为 warn，5 及以上记为 error。
func (l *frameworkLogger) emit(ctx context.Context, level int) {
	if level < int(l.level.Load()) {
		return
	}
	_, file, line, _ := runtime.Caller(2)
	severity := slog.LevelInfo
	if level >= 4 {
		severity = slog.LevelWarn
	}
	if level >= 5 {
		severity = slog.LevelError
	}
	contextualLogger{ctx: ctx}.write(severity, "framework diagnostic", Field("component", l.component), Field("source", filepath.Base(file)), Field("line", line))
}

// SetOutput 让框架改写输出时也走统一的日志 writer。
func (l *frameworkLogger) SetOutput(w io.Writer) { SetWriter(w) }

// KitexLogger 适配 klog.FullLogger。
type KitexLogger struct{ frameworkLogger }

// SetLevel 设置 Kitex 框架日志的最低级别。
func (l *KitexLogger) SetLevel(v klog.Level) { l.level.Store(int32(v)) }

// HertzLogger 适配 hlog.FullLogger。
type HertzLogger struct{ frameworkLogger }

// SetLevel 设置 Hertz 框架日志的最低级别。
func (l *HertzLogger) SetLevel(v hlog.Level) { l.level.Store(int32(v)) }

// ConfigureFrameworks 把 Kitex 与 Hertz 的日志接入统一输出，默认只保留 warn 及以上。
func ConfigureFrameworks() {
	k := &KitexLogger{frameworkLogger: frameworkLogger{component: "kitex"}}
	k.SetLevel(klog.LevelWarn)
	klog.SetLogger(k)
	h := &HertzLogger{frameworkLogger: frameworkLogger{component: "hertz"}}
	h.SetLevel(hlog.LevelWarn)
	hlog.SetLogger(h)
}

// 以下方法实现框架日志接口，按 trace(0) 到 error(5) 的级别转发给 emit。
func (l *frameworkLogger) Trace(_ ...any)                                     { l.emit(context.Background(), 0) }
func (l *frameworkLogger) Tracef(_ string, _ ...any)                          { l.emit(context.Background(), 0) }
func (l *frameworkLogger) CtxTracef(ctx context.Context, _ string, _ ...any)  { l.emit(ctx, 0) }
func (l *frameworkLogger) Debug(_ ...any)                                     { l.emit(context.Background(), 1) }
func (l *frameworkLogger) Debugf(_ string, _ ...any)                          { l.emit(context.Background(), 1) }
func (l *frameworkLogger) CtxDebugf(ctx context.Context, _ string, _ ...any)  { l.emit(ctx, 1) }
func (l *frameworkLogger) Info(_ ...any)                                      { l.emit(context.Background(), 2) }
func (l *frameworkLogger) Infof(_ string, _ ...any)                           { l.emit(context.Background(), 2) }
func (l *frameworkLogger) CtxInfof(ctx context.Context, _ string, _ ...any)   { l.emit(ctx, 2) }
func (l *frameworkLogger) Notice(_ ...any)                                    { l.emit(context.Background(), 3) }
func (l *frameworkLogger) Noticef(_ string, _ ...any)                         { l.emit(context.Background(), 3) }
func (l *frameworkLogger) CtxNoticef(ctx context.Context, _ string, _ ...any) { l.emit(ctx, 3) }
func (l *frameworkLogger) Warn(_ ...any)                                      { l.emit(context.Background(), 4) }
func (l *frameworkLogger) Warnf(_ string, _ ...any)                           { l.emit(context.Background(), 4) }
func (l *frameworkLogger) CtxWarnf(ctx context.Context, _ string, _ ...any)   { l.emit(ctx, 4) }
func (l *frameworkLogger) Error(_ ...any)                                     { l.emit(context.Background(), 5) }
func (l *frameworkLogger) Errorf(_ string, _ ...any)                          { l.emit(context.Background(), 5) }
func (l *frameworkLogger) CtxErrorf(ctx context.Context, _ string, _ ...any)  { l.emit(ctx, 5) }

// Fatal 系列记录后 panic，框架致命错误不能被静默吞掉。
func (l *frameworkLogger) Fatal(_ ...any) {
	l.emit(context.Background(), 6)
	panic("fatal framework diagnostic")
}
func (l *frameworkLogger) Fatalf(_ string, _ ...any) {
	l.emit(context.Background(), 6)
	panic("fatal framework diagnostic")
}
func (l *frameworkLogger) CtxFatalf(ctx context.Context, _ string, _ ...any) {
	l.emit(ctx, 6)
	panic("fatal framework diagnostic")
}
