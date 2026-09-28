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

type frameworkLogger struct {
	component string
	level     atomic.Int32
}

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
func (l *frameworkLogger) SetOutput(w io.Writer) { SetWriter(w) }

type KitexLogger struct{ frameworkLogger }

func (l *KitexLogger) SetLevel(v klog.Level) { l.level.Store(int32(v)) }

type HertzLogger struct{ frameworkLogger }

func (l *HertzLogger) SetLevel(v hlog.Level) { l.level.Store(int32(v)) }
func ConfigureFrameworks() {
	k := &KitexLogger{frameworkLogger: frameworkLogger{component: "kitex"}}
	k.SetLevel(klog.LevelWarn)
	klog.SetLogger(k)
	h := &HertzLogger{frameworkLogger: frameworkLogger{component: "hertz"}}
	h.SetLevel(hlog.LevelWarn)
	hlog.SetLogger(h)
}
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
