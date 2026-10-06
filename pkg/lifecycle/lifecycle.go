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

// initContext lazily creates the process context that SIGINT/SIGTERM cancel.
func initContext() {
	once.Do(func() { ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) })
}

// Done is closed when the process is asked to stop; Shutdown requests a stop programmatically;
// AddShutdownListener runs fn once the stop is requested.
func Done() <-chan struct{} { initContext(); return ctx.Done() }

// Shutdown requests a stop programmatically.
func Shutdown()                     { initContext(); stop() }
func AddShutdownListener(fn func()) { go func() { <-Done(); fn() }() }

// ServiceConf is the common service block: name, logging, mode and the diagnostics server.
type ServiceConf struct {
	Name       string
	Log        logging.Config
	Mode       string `json:",default=pro,options=dev|test|rt|pre|pro"`
	DevServer  diagnostics.ServerConfig
	Prometheus diagnostics.Config
}

// MustSetUp panics when SetUp fails (startup only).
func (c ServiceConf) MustSetUp() {
	if err := c.SetUp(); err != nil {
		panic(err)
	}
}

// SetUp configures logging and starts the diagnostics server; a legacy Prometheus port
// alone still enables the metrics endpoint.
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
