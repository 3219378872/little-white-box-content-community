// Package diagnostics serves private health and Prometheus endpoints.
package diagnostics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config is the legacy Prometheus endpoint block kept for older service configs.
type Config struct {
	Host string
	Port int
	Path string
}

// ServerConfig enables the internal diagnostics server and its optional endpoints.
type ServerConfig struct {
	Enabled       bool
	Host          string
	Port          int
	EnableMetrics bool
	EnablePprof   bool
}

// Start serves /healthz plus optional /metrics and pprof on a separate listener and returns
// a function that shuts it down within five seconds.
func Start(c ServerConfig) (func(), error) {
	if !c.Enabled {
		return func() {}, nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	if c.EnableMetrics {
		mux.Handle("/metrics", promhttp.Handler())
	}
	if c.EnablePprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(c.Host, fmt.Sprint(c.Port)))
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}, nil
}
