package middleware

import (
	"context"
	"esx/pkg/errx"
	"esx/pkg/httpx"
	"esx/pkg/logging"
	"esx/pkg/rpcx"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/prometheus/client_golang/prometheus"
)

func (m *TraceMiddleware) Hertz(ctx context.Context, c *app.RequestContext) {
	id := strings.TrimSpace(string(c.GetHeader("X-Trace-ID")))
	if id == "" {
		id = strings.TrimSpace(string(c.GetHeader("X-Request-ID")))
	}
	if id == "" {
		id = newTraceID()
	}
	ctx = rpcx.WithTraceID(ctx, id)
	ctx = context.WithValue(ctx, clientIPKey{}, hertzClientIP(c))
	c.Header("X-Trace-ID", id)
	c.Next(ctx)
}
func hertzClientIP(c *app.RequestContext) string {
	if f := string(c.GetHeader("X-Forwarded-For")); f != "" {
		if ip := strings.TrimSpace(strings.Split(f, ",")[0]); ip != "" {
			return ip
		}
	}
	if ip := strings.TrimSpace(string(c.GetHeader("X-Real-IP"))); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err == nil {
		return host
	}
	return c.RemoteAddr().String()
}

var httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "esx", Subsystem: "gateway", Name: "requests_total", Help: "HTTP responses by route and status"}, []string{"method", "route", "status"})
var httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "esx", Subsystem: "gateway", Name: "duration_seconds", Help: "HTTP lifetime through the final response frame"}, []string{"method", "route"})

func init() { prometheus.MustRegister(httpRequests, httpDuration) }
func (m *SafeAccessLogMiddleware) Hertz(ctx context.Context, c *app.RequestContext) {
	started := time.Now()
	c.Next(ctx)
	method, route := string(c.Method()), c.FullPath()
	if route == "" {
		route = "unmatched"
		method = "OTHER"
	}
	httpRequests.WithLabelValues(method, route, strconv.Itoa(c.Response.StatusCode())).Inc()
	httpDuration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
	logging.WithContext(ctx).Infow("gateway request completed", logging.Field("method", method), logging.Field("path", route), logging.Field("status", c.Response.StatusCode()), logging.Field("duration_ms", time.Since(started).Milliseconds()))
}
func (m *BehaviorAcceptedMiddleware) Hertz(ctx context.Context, c *app.RequestContext) {
	requestID := strings.TrimSpace(string(c.GetHeader(requestIDHeader)))
	if requestID == "" {
		requestID = newRequestID()
	}
	traceID := strings.TrimSpace(string(c.GetHeader(traceIDHeader)))
	if traceID == "" {
		traceID = requestID
	}
	metadata := BehaviorRequestMetadata{ClientIP: hertzClientIP(c), UserAgent: string(c.Request.Header.UserAgent()), ClientVersion: strings.TrimSpace(string(c.GetHeader(clientVersionHeader))), TraceID: traceID}
	c.Header(requestIDHeader, requestID)
	c.Next(context.WithValue(ctx, behaviorMetadataKey{}, metadata))
	if c.Response.StatusCode() == 200 {
		c.Status(202)
	}
}
func Recovery(ctx context.Context, c *app.RequestContext) {
	defer func() {
		if recover() != nil {
			logging.WithContext(ctx).Errorw("gateway handler panic")
			if c.Response.GetHijackWriter() == nil {
				httpx.ErrorCtx(ctx, c, errx.NewWithCode(errx.SystemError))
			}
			c.Abort()
		}
	}()
	c.Next(ctx)
}
