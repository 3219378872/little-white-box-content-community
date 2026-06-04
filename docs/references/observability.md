# Observability Patterns

## Table of Contents
- [Architecture Overview](#architecture-overview)
- [1. Distributed Tracing](#1-distributed-tracing)
- [2. Metrics](#2-metrics)
- [3. Structured Logging](#3-structured-logging)
- [4. Health Checks](#4-health-checks)
- [5. Graceful Shutdown](#5-graceful-shutdown)
- [6. Prometheus Scrape & AlertManager Configuration](#6-prometheus-scrape--alertmanager-configuration)
- [7. Log Rotation & Retention](#7-log-rotation--retention)
- [8. Metrics Cardinality Management](#8-metrics-cardinality-management)
- [Best Practices Summary](#best-practices-summary)

Three pillars of observability for go-zero microservices: Tracing, Metrics, and Logging.

## Architecture Overview

```
Service → OpenTelemetry SDK → OTLP Exporter → Collector → Backend
                                                  ├── Jaeger (Traces)
                                                  ├── Prometheus (Metrics)
                                                  └── ELK/Loki (Logs)
```

---

## 1. Distributed Tracing

### ✅ Built-in Telemetry Configuration

go-zero has built-in OpenTelemetry support:

```yaml
# etc/service.yaml
Telemetry:
  Name: user-api
  Endpoint: http://jaeger:14268/api/traces  # Jaeger collector
  Sampler: 1.0   # 1.0 = sample all, 0.1 = sample 10%
  Batcher: jaeger # jaeger or otlpgrpc or otlphttp
```

With this config, go-zero automatically:
- Creates spans for all HTTP handlers
- Creates spans for all RPC calls (client and server)
- Propagates trace context across services via gRPC metadata / HTTP headers
- Records span duration, status, and errors

### ✅ Custom Span Creation

For business logic that needs sub-spans:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
)

// Cache tracer at package level — otel.Tracer() is cheap but should not be called per-request
var orderTracer = otel.Tracer("order-service")

func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    tracer := orderTracer

    // Create span for business logic
    ctx, span := tracer.Start(l.ctx, "CreateOrder.ValidateAndProcess")
    defer span.End()

    // Add business attributes
    span.SetAttributes(
        attribute.Int64("user.id", req.UserId),
        attribute.Float64("order.amount", req.Amount),
        attribute.Int("order.items_count", len(req.Items)),
    )

    // Sub-operation span
    ctx2, validateSpan := tracer.Start(ctx, "CreateOrder.ValidateInventory")
    err := l.validateInventory(ctx2, req.Items)
    validateSpan.End()

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return nil, err
    }

    span.SetStatus(codes.Ok, "order created")
    return &types.CreateOrderResponse{OrderId: orderId}, nil
}
```

### ✅ Trace Context Propagation

go-zero automatically propagates trace context for:
- REST → RPC calls (via gRPC metadata)
- RPC → RPC calls (via gRPC metadata)
- REST → REST calls (via HTTP headers)

For manual propagation (e.g., message queues):

```go
import (
    "go.opentelemetry.io/otel/propagation"
)

// Inject trace context into message headers
func injectTraceContext(ctx context.Context, headers map[string]string) {
    propagator := otel.GetTextMapPropagator()
    carrier := propagation.MapCarrier(headers)
    propagator.Inject(ctx, carrier)
}

// Extract trace context from message headers
func extractTraceContext(ctx context.Context, headers map[string]string) context.Context {
    propagator := otel.GetTextMapPropagator()
    carrier := propagation.MapCarrier(headers)
    return propagator.Extract(ctx, carrier)
}
```

---

## 2. Metrics

### ✅ Built-in Prometheus Metrics

```yaml
# etc/service.yaml
Prometheus:
  Host: 0.0.0.0
  Port: 9091
  Path: /metrics
```

go-zero automatically exports:
- `http_server_requests_duration_ms` — HTTP request duration histogram
- `http_server_requests_code_total` — HTTP response code counter
- `rpc_server_requests_duration_ms` — RPC request duration
- `rpc_server_requests_code_total` — RPC response code counter

### ✅ Custom Business Metrics

```go
import (
    "github.com/zeromicro/go-zero/core/metric"
)

var (
    // Counter
    orderCreatedCounter = metric.NewCounterVec(&metric.CounterVecOpts{
        Namespace: "business",
        Subsystem: "order",
        Name:      "created_total",
        Help:      "Total number of orders created",
        Labels:    []string{"status", "payment_method"},
    })

    // Histogram
    orderAmountHistogram = metric.NewHistogramVec(&metric.HistogramVecOpts{
        Namespace: "business",
        Subsystem: "order",
        Name:      "amount_yuan",
        Help:      "Order amount distribution in yuan",
        Labels:    []string{"category"},
        Buckets:   []float64{10, 50, 100, 500, 1000, 5000},
    })

    // Gauge
    activeUsersGauge = metric.NewGaugeVec(&metric.GaugeVecOpts{
        Namespace: "business",
        Subsystem: "user",
        Name:      "active_count",
        Help:      "Number of currently active users",
        Labels:    []string{"region"},
    })
)

func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    resp, err := l.doCreateOrder(req)
    if err != nil {
        orderCreatedCounter.Inc("failed", req.PaymentMethod)
        return nil, err
    }

    orderCreatedCounter.Inc("success", req.PaymentMethod)
    orderAmountHistogram.Observe(float64(req.Amount), req.Category)
    return resp, nil
}
```

### ✅ SLI/SLO Monitoring

Define Service Level Indicators and Objectives:

```yaml
# Prometheus alerting rules (prometheus-rules.yaml)
groups:
  - name: slo-alerts
    rules:
      # Availability SLO: 99.9% success rate
      - alert: HighErrorRate
        expr: |
          (
            sum(rate(http_server_requests_code_total{code=~"5.."}[5m]))
            /
            sum(rate(http_server_requests_code_total[5m]))
          ) > 0.001
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Error rate exceeds 0.1% SLO (current: {{ $value | humanizePercentage }})"

      # Latency SLO: P99 < 500ms
      - alert: HighLatencyP99
        expr: |
          histogram_quantile(0.99,
            sum(rate(http_server_requests_duration_ms_bucket[5m])) by (le, path)
          ) > 500
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P99 latency exceeds 500ms SLO on {{ $labels.path }}"

      # Saturation: CPU > 80%
      - alert: HighCPUUsage
        expr: process_cpu_seconds_total > 0.8
        for: 10m
        labels:
          severity: warning
```

---

## 3. Structured Logging

### ✅ Trace-Correlated Logging

go-zero's logx automatically includes trace ID when Telemetry is configured:

```go
func (l *CreateOrderLogic) CreateOrder(req *types.CreateOrderRequest) (*types.CreateOrderResponse, error) {
    // Trace ID is automatically injected into log output
    l.Logger.Infow("creating order",
        logx.Field("user_id", req.UserId),
        logx.Field("amount", req.Amount),
        logx.Field("items_count", len(req.Items)),
    )

    // Error logs also carry trace context
    if err != nil {
        l.Logger.WithFields(
            logx.Field("user_id", req.UserId),
            logx.Field("error_type", "payment_failed"),
        ).Errorf("order creation failed: %v", err)
    }

    return resp, nil
}
```

Log output (JSON mode) automatically includes `trace` and `span`:

```json
{
  "@timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "content": "creating order",
  "trace": "abc123def456",
  "span": "789xyz",
  "user_id": 12345,
  "amount": 99.99,
  "items_count": 3
}
```

### ✅ Log Configuration for Production

```yaml
Log:
  Mode: file          # file for production, console for dev
  Encoding: json      # json for machine parsing, plain for human reading
  Level: info         # debug, info, error, severe
  Path: /var/log/app
  Compress: true
  KeepDays: 7
  MaxSize: 200        # MB per file
  Rotation: daily     # daily or size-based
```

### ✅ Sensitive Data Masking

```go
// Never log these directly
// ❌ l.Logger.Infof("password: %s", password)
// ❌ l.Logger.Infof("token: %s", token)
// ❌ l.Logger.Infof("credit_card: %s", ccNumber)

// ✅ Mask sensitive fields
func maskEmail(email string) string {
    parts := strings.Split(email, "@")
    if len(parts) != 2 {
        return "***"
    }
    name := parts[0]
    if len(name) > 2 {
        name = name[:2] + "***"
    }
    return name + "@" + parts[1]
}

l.Logger.Infof("user login: %s", maskEmail(req.Email))
```

---

## 4. Health Checks

### ✅ Health Check Endpoint

```go
// handler/healthhandler.go
func HealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        checks := map[string]string{}
        healthy := true

        // Check database (RawDB may return nil if connection pool is not initialized)
        if rawDB := svcCtx.DB.RawDB(); rawDB == nil {
            checks["database"] = "unhealthy: connection pool not initialized"
            healthy = false
        } else if err := rawDB.PingContext(r.Context()); err != nil {
            checks["database"] = "unhealthy: " + err.Error()
            healthy = false
        } else {
            checks["database"] = "healthy"
        }

        // Check Redis
        if err := svcCtx.Redis.PingCtx(r.Context()); err != nil {
            checks["redis"] = "unhealthy: " + err.Error()
            healthy = false
        } else {
            checks["redis"] = "healthy"
        }

        status := http.StatusOK
        if !healthy {
            status = http.StatusServiceUnavailable
        }

        httpx.WriteJsonCtx(r.Context(), w, status, map[string]interface{}{
            "status": map[bool]string{true: "healthy", false: "unhealthy"}[healthy],
            "checks": checks,
        })
    }
}
```

Register in routes:

```go
// Register health check (no auth required)
server.AddRoute(rest.Route{
    Method:  http.MethodGet,
    Path:    "/health",
    Handler: handler.HealthHandler(serverCtx),
})
```

---

## 5. Graceful Shutdown

### ✅ Built-in Signal Handling

go-zero handles SIGTERM/SIGINT automatically. The `rest.MustNewServer` and `zrpc.MustNewServer` register shutdown hooks:

```go
func main() {
    var c config.Config
    conf.MustLoad(*configFile, &c)

    server := rest.MustNewServer(c.RestConf)
    defer server.Stop() // Graceful shutdown: stops accepting new requests, waits for in-flight

    // Register custom cleanup
    proc.AddShutdownListener(func() {
        logx.Info("shutting down, cleaning up resources...")
        // Close custom connections, flush buffers, etc.
    })

    server.Start()
}
```

### ✅ Kubernetes Graceful Shutdown

```yaml
spec:
  terminationGracePeriodSeconds: 30  # Give pods 30s to drain
  containers:
    - name: service
      lifecycle:
        preStop:
          exec:
            command: ["/bin/sh", "-c", "sleep 5"]  # Wait for endpoint removal to propagate
```

---

## 6. Prometheus Scrape & AlertManager Configuration

### ✅ Prometheus Scrape Config

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "rules/*.yml"

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

scrape_configs:
  # Scrape go-zero services via Kubernetes service discovery
  - job_name: 'go-zero-services'
    kubernetes_sd_configs:
      - role: pod
    relabel_configs:
      # Only scrape pods with annotation prometheus.io/scrape: "true"
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      # Use custom port if annotated
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        target_label: __address__
        regex: (.+)
        replacement: ${1}:${2}
      # Add service name label
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: replace
        target_label: service

  # Direct scrape for non-K8s environments
  - job_name: 'user-api'
    static_configs:
      - targets: ['user-api:9101']
    metrics_path: /metrics
```

### ✅ go-zero Service Prometheus Annotations

```yaml
# K8s deployment — add annotations for auto-discovery
spec:
  template:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9101"
        prometheus.io/path: "/metrics"
```

### ✅ AlertManager Configuration

```yaml
# alertmanager.yml
global:
  resolve_timeout: 5m

route:
  receiver: 'default'
  group_by: ['alertname', 'service']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h

  routes:
    # Critical alerts → PagerDuty + Slack immediately
    - match:
        severity: critical
      receiver: 'pagerduty-critical'
      group_wait: 10s
      repeat_interval: 1h

    # Warning alerts → Slack
    - match:
        severity: warning
      receiver: 'slack-warning'
      repeat_interval: 4h

receivers:
  - name: 'default'
    webhook_configs:
      - url: 'http://alertmanager-webhook:5001/'

  - name: 'pagerduty-critical'
    pagerduty_configs:
      - routing_key: '${PAGERDUTY_ROUTING_KEY}'
        severity: critical

  - name: 'slack-warning'
    slack_configs:
      - api_url: '${SLACK_WEBHOOK_URL}'
        channel: '#alerts-warning'
        title: '{{ .GroupLabels.alertname }}'
        text: >-
          {{ range .Alerts }}
          *Service*: {{ .Labels.service }}
          *Summary*: {{ .Annotations.summary }}
          *Description*: {{ .Annotations.description }}
          {{ end }}

# Inhibition: suppress warning when critical is firing for same service
inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'service']
```

### ✅ Alert Rules for go-zero Services

```yaml
# rules/go-zero-alerts.yml
groups:
  - name: go-zero-slo
    rules:
      # SLO: 99.9% availability
      - alert: HighErrorRate
        expr: |
          sum(rate(http_server_requests_total{code=~"5.."}[5m])) by (service)
          /
          sum(rate(http_server_requests_total[5m])) by (service)
          > 0.001
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.service }} error rate exceeds SLO (0.1%)"
          description: "Current error rate: {{ $value | humanizePercentage }}"

      # SLO: p99 latency < 500ms
      - alert: HighLatency
        expr: |
          histogram_quantile(0.99, sum(rate(http_server_request_duration_seconds_bucket[5m])) by (service, le))
          > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "{{ $labels.service }} p99 latency > 500ms"
          description: "Current p99: {{ $value | humanizeDuration }}"

      # Pod restart detection
      - alert: PodCrashLooping
        expr: rate(kube_pod_container_status_restarts_total[15m]) * 60 * 15 > 3
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Pod {{ $labels.pod }} is crash looping"

      # Goroutine leak detection
      - alert: GoroutineLeak
        expr: go_goroutines > 10000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "{{ $labels.service }} has {{ $value }} goroutines — possible leak"
```

---

## 7. Log Rotation & Retention

### ✅ go-zero Log Configuration

```yaml
# etc/service.yaml
Log:
  ServiceName: user-api
  Mode: file           # "console" | "file" | "volume"
  Path: /var/log/user-api
  Level: info          # debug | info | error | severe
  KeepDays: 7          # Auto-delete logs older than 7 days
  MaxBackups: 0        # 0 = unlimited backups (managed by KeepDays)
  MaxSize: 0           # 0 = no size limit per file (rotated daily)
  Rotation: daily      # "daily" — go-zero rotates by date
  Compress: false      # Compress rotated logs (gzip)
  Encoding: json       # "json" | "plain" — always use JSON in production
```

### ✅ Kubernetes Log Strategy

```yaml
# For container-based deployments, go-zero logs to stdout (Mode: console)
# and relies on the cluster's log aggregation pipeline:
#
# Container stdout → Fluentd/Fluent Bit → Elasticsearch/Loki → Kibana/Grafana
#
# Recommended: Mode: console in K8s, Mode: file only in VM deployments

Log:
  Mode: console        # stdout/stderr for K8s log collection
  Encoding: json       # Machine-parseable for log pipeline
  Level: info
```

### ✅ Retention Policy Guidelines

```
Environment     │ Retention  │ Rationale
────────────────┼────────────┼──────────────────────────────
Development     │ 1 day      │ Minimize disk usage
Staging         │ 7 days     │ Enough for debugging
Production      │ 30 days    │ Compliance + incident investigation
Audit logs      │ 1+ years   │ Regulatory requirement (store in object storage)
```

### ✅ External Log Rotation (logrotate)

For VM deployments where go-zero's built-in rotation is insufficient:

```bash
# /etc/logrotate.d/go-zero
/var/log/go-zero/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    copytruncate        # Don't interrupt running process
    dateext
    dateformat -%Y%m%d
    maxsize 500M        # Rotate early if file exceeds 500MB
}
```

---

## 8. Metrics Cardinality Management

High cardinality labels (user_id, request_id, trace_id) cause memory explosion in Prometheus. This section covers prevention and mitigation.

### ❌ Anti-Pattern: High Cardinality Labels

```go
// BAD — user_id has unbounded cardinality → millions of time series
httpRequestsTotal.WithLabelValues(method, path, statusCode, userId).Inc()

// BAD — full URL path with IDs → /api/v1/users/12345 is a unique label value
httpRequestsTotal.WithLabelValues(method, r.URL.Path, statusCode).Inc()
```

### ✅ Correct: Bounded Labels Only

```go
// GOOD — use route template, not actual path
httpRequestsTotal.WithLabelValues(method, routeTemplate, statusCode).Inc()

// routeTemplate = "/api/v1/users/:id" (bounded)
// NOT r.URL.Path = "/api/v1/users/12345" (unbounded)
```

### ✅ Label Cardinality Checklist

```
Before adding a metric label, ask:
1. How many unique values can this label have?
   - < 100: Safe ✅ (method, status_code, service_name)
   - 100-1000: Caution ⚠️ (endpoint, error_type)
   - > 1000: Danger ❌ (user_id, request_id, ip_address)

2. Does it grow with traffic or users?
   - No: Safe ✅ (version, region, env)
   - Yes: Danger ❌ (session_id, order_id)

Rule of thumb: total_series = metric_count × label_1_cardinality × label_2_cardinality × ...
Keep total_series < 100,000 per service.
```

### ✅ Relabeling to Drop High Cardinality

```yaml
# prometheus.yml — drop dangerous labels at scrape time
scrape_configs:
  - job_name: 'go-zero-services'
    metric_relabel_configs:
      # Drop any metric with user_id label (accidental exposure)
      - source_labels: [user_id]
        regex: '.+'
        action: drop
      # Drop high-cardinality path labels, keep route template
      - source_labels: [path]
        regex: '/api/v[0-9]+/[a-z]+/[0-9]+'
        action: drop
```

### ✅ Monitor Cardinality in Grafana

```promql
# Top 10 metrics by series count — run periodically to detect cardinality issues
topk(10, count by (__name__)({__name__=~".+"}))

# Services with the most time series
topk(10, count by (service)({__name__=~".+"}))

# Alert on cardinality explosion
count({__name__=~"http_.*"}) > 50000
```

---

## Best Practices Summary

### ✅ DO:
- Enable Telemetry in production config for automatic tracing
- Set appropriate sampling rate (0.1-1.0 based on traffic volume)
- Use structured logging (JSON) with trace correlation
- Define SLOs and configure alerts for SLI violations
- Create custom business metrics for domain-specific monitoring
- Implement health check endpoints with dependency checks
- Mask sensitive data (PII, tokens, passwords) in logs
- Use `proc.AddShutdownListener` for custom cleanup on shutdown

### ❌ DON'T:
- Log sensitive information (passwords, tokens, credit cards)
- Use high cardinality labels in metrics (user_id, request_id)
- Sample at 100% in high-traffic production (use 0.01-0.1)
- Expose Prometheus metrics endpoint without authentication
- Ignore trace context when publishing to message queues
- Skip health checks for readiness probes — they prevent traffic to unhealthy pods

For resilience patterns (circuit breaker, rate limiting), see [Resilience Patterns](./resilience-patterns.md).
For database monitoring, see [Database Patterns](./database-patterns.md).
