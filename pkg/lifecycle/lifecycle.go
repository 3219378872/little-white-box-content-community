// Package lifecycle owns process cancellation and diagnostic server shutdown.
package lifecycle

import (
	"context"
	"esx/pkg/diagnostics"
	"esx/pkg/logging"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

const (
	DevMode  = "dev"
	TestMode = "test"
)

var once sync.Once
var ctx context.Context
var stop context.CancelFunc

func initContext() {
	once.Do(func() { ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) })
}
func Done() <-chan struct{}         { initContext(); return ctx.Done() }
func Shutdown()                     { initContext(); stop() }
func AddShutdownListener(fn func()) { go func() { <-Done(); fn() }() }

type ServiceConf struct {
	Name       string
	Log        logging.Config
	Mode       string `json:",default=pro,options=dev|test|rt|pre|pro"`
	DevServer  diagnostics.ServerConfig
	Prometheus diagnostics.Config
}

func (c ServiceConf) MustSetUp() {
	if err := c.SetUp(); err != nil {
		panic(err)
	}
}
func (c ServiceConf) SetUp() error {
	if err := logging.Configure(c.Log); err != nil {
		return err
	}
	d := c.DevServer
	if !d.Enabled && c.Prometheus.Port > 0 {
		d = diagnostics.ServerConfig{Enabled: true, Host: c.Prometheus.Host, Port: c.Prometheus.Port, EnableMetrics: true}
	}
	closeFn, err := diagnostics.Start(d)
	if err != nil {
		return err
	}
	var closed sync.Once
	closeDiagnostics := func() { closed.Do(closeFn) }
	TrackResource(closeResourceFunc(closeDiagnostics))
	AddShutdownListener(closeDiagnostics)
	return nil
}
