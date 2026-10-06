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

// Hertz 透传或生成追踪 ID，并把客户端 IP 放入 ctx 供下游频控使用（REL-052）。
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

// hertzClientIP 依次取 X-Forwarded-For 首跳、X-Real-IP 与连接对端地址（生产由 nginx 覆写可信头）。
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

// 网关 HTTP 指标按路由模板而非原始路径打标签，避免标签基数随 ID 增长。
var httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "esx", Subsystem: "gateway", Name: "requests_total", Help: "HTTP responses by route and status"}, []string{"method", "route", "status"})
var httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "esx", Subsystem: "gateway", Name: "duration_seconds", Help: "HTTP lifetime through the final response frame"}, []string{"method", "route"})

// init 在进程启动时注册网关指标。
func init() { prometheus.MustRegister(httpRequests, httpDuration) }

// Hertz 在响应结束后记录指标与访问日志；只记录方法、路由模板与状态，不记录查询串和请求体。
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

// Hertz 补齐行为请求元数据并回写请求 ID；下游成功时把 200 改写为 202。
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

// Recovery 把处理器 panic 转成 SystemError；已劫持的流式响应无法再写错误体，只中止请求。
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
