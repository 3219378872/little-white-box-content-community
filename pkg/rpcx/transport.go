package rpcx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"esx/pkg/errx"
	"esx/pkg/lifecycle"
	"esx/pkg/logging"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/kitex/pkg/registry"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/circuitbreak"
	"github.com/cloudwego/kitex/pkg/discovery"
	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/limit"
	"github.com/cloudwego/kitex/pkg/remote"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/codes"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/status"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/pkg/serviceinfo"
	"github.com/cloudwego/kitex/pkg/streaming"
	"github.com/cloudwego/kitex/server"
	"github.com/cloudwego/kitex/transport"
	etcd "github.com/kitex-contrib/registry-etcd"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	grpcstatus "google.golang.org/grpc/status"
)

// WithTraceID stores the trace ID propagated to downstream RPCs.
func WithTraceID(ctx context.Context, id string) context.Context {
	return logging.WithTraceID(ctx, id)
}

// TraceID returns the propagated trace ID.
func TraceID(ctx context.Context) string { return logging.TraceID(ctx) }

var calls = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "esx", Subsystem: "rpc", Name: "requests_total", Help: "RPC requests by side, method and outcome"}, []string{"side", "method", "code"})
var latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "esx", Subsystem: "rpc", Name: "duration_seconds", Help: "RPC duration through response completion"}, []string{"side", "method"})

// init registers RPC metrics and routes framework logs through pkg/logging.
func init() { prometheus.MustRegister(calls, latency); logging.ConfigureFrameworks() }

// fullMethod renders the gRPC method path (/package.Service/Method) that is signed and labelled.
func fullMethod(ctx context.Context) string {
	i := rpcinfo.GetRPCInfo(ctx).Invocation()
	name := i.ServiceName()
	if i.PackageName() != "" {
		name = i.PackageName() + "." + name
	}
	return "/" + name + "/" + i.MethodName()
}

// signature is the HMAC-SHA256 of timestamp and method under the shared internal secret.
func signature(secret, ts, method string) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write([]byte(ts))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(method))
	return hex.EncodeToString(h.Sum(nil))
}

// exempt lets health checks through without internal credentials.
func exempt(method string) bool { return strings.HasPrefix(method, "/grpc.health.v1.Health/") }

// authClient signs every outgoing call with a timestamp and method signature and forwards the trace ID.
func authClient(secret string) func(context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		method := fullMethod(ctx)
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		if md == nil {
			md = metadata.MD{}
		}
		if secret != "" && !exempt(method) {
			ts := strconv.FormatInt(time.Now().Unix(), 10)
			md.Set("x-internal-timestamp", ts)
			md.Set("x-internal-signature", signature(secret, ts, method))
		}
		if id := TraceID(ctx); id != "" {
			md.Set("trace_id", id)
		}
		return metadata.NewOutgoingContext(ctx, md), nil
	}
}

// authServer rejects calls without a valid internal signature or with a timestamp more than
// five minutes off, then restores the caller's trace ID.
func authServer(secret string) func(context.Context) (context.Context, error) {
	return func(ctx context.Context) (context.Context, error) {
		method := fullMethod(ctx)
		if exempt(method) {
			return ctx, nil
		}
		md, _ := metadata.FromIncomingContext(ctx)
		ts, sig := md.Get("x-internal-timestamp"), md.Get("x-internal-signature")
		if len(ts) != 1 || len(sig) != 1 {
			return ctx, status.Err(codes.Unauthenticated, "internal credentials missing")
		}
		n, e := strconv.ParseInt(ts[0], 10, 64)
		if e != nil {
			return ctx, status.Err(codes.Unauthenticated, "internal timestamp malformed")
		}
		skew := time.Since(time.Unix(n, 0))
		if skew > 5*time.Minute || skew < -5*time.Minute {
			return ctx, status.Err(codes.Unauthenticated, "internal timestamp expired")
		}
		if !hmac.Equal([]byte(signature(secret, ts[0], method)), []byte(sig[0])) {
			return ctx, status.Err(codes.PermissionDenied, "internal signature mismatch")
		}
		if ids := md.Get("trace_id"); len(ids) == 1 {
			ctx = WithTraceID(ctx, ids[0])
		}
		return ctx, nil
	}
}

// ToTransportError converts a handler error into a gRPC status carrying the business code;
// unknown errors become SystemError so internals do not leak.
func ToTransportError(err error) error {
	if err == nil {
		return nil
	}
	var native interface{ GRPCStatus() *status.Status }
	if errors.As(err, &native) {
		return native.GRPCStatus().Err()
	}
	if s, ok := grpcstatus.FromError(err); ok {
		return status.FromProto(grpcstatus.Convert(errx.FromGRPCError(s.Err())).Proto()).Err()
	}
	if errors.Is(err, context.Canceled) {
		return status.Err(codes.Canceled, "request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Err(codes.DeadlineExceeded, "request timed out")
	}
	return status.FromProto(grpcstatus.Convert(errx.NewWithCode(errx.SystemError)).Proto()).Err()
}

// FromTransportError converts a received gRPC status back into an errx business error;
// EOF and cancellation pass through for stream handling.
func FromTransportError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return err
	}
	if err == nil {
		return nil
	}
	var native interface{ GRPCStatus() *status.Status }
	if errors.As(err, &native) {
		return errx.FromGRPCError(grpcstatus.FromProto(native.GRPCStatus().Proto()).Err())
	}
	return errx.FromRPCError(err)
}

// A Kitex tracer finishes with the stream, so latency includes all frames.
type observationKey struct{ side string }

// observer records per-call metrics and failure logs for one side (client or server).
type observer struct{ side string }

// Start records the start time.
func (o observer) Start(ctx context.Context) context.Context {
	return context.WithValue(ctx, observationKey(o), time.Now())
}

// Finish counts the call, observes latency and logs failures with method and duration.
func (o observer) Finish(ctx context.Context) {
	started, ok := ctx.Value(observationKey(o)).(time.Time)
	if !ok {
		return
	}
	info := rpcinfo.GetRPCInfo(ctx)
	if info == nil {
		return
	}
	err := info.Stats().Error()
	method := fullMethod(ctx)
	code := "OK"
	if err != nil {
		code = "error"
	}
	calls.WithLabelValues(o.side, method, code).Inc()
	latency.WithLabelValues(o.side, method).Observe(time.Since(started).Seconds())
	if err != nil {
		logging.WithContext(ctx).Errorw("rpc call failed", logging.Field("side", o.side), logging.Field("method", method), logging.Field("duration_ms", time.Since(started).Milliseconds()))
	}
}

// observed maps errors at the RPC boundary: servers encode business errors, clients decode them.
func observed(side string) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp any) error {
			err := next(ctx, req, resp)
			if side == "server" {
				return ToTransportError(err)
			}
			return FromTransportError(err)
		}
	}
}

// ClientOption customizes NewClient.
type ClientOption func(*clientSpec)

// Client is a service client definition: Kitex options, health probing and owned resources.
type Client interface {
	Options() []client.Option
	ServiceName() string
	Probe(context.Context) error
	Track(io.Closer)
	Close() error
}

// clientSpec is the Client implementation; closers are released with the client.
type clientSpec struct {
	conf     RpcClientConf
	secret   string
	resolver discovery.Resolver
	mu       sync.Mutex
	closers  []io.Closer
	cancel   context.CancelFunc
}

// WithInternalAuth signs outgoing calls; an empty secret is a startup error.
func WithInternalAuth(secret string) ClientOption {
	if strings.TrimSpace(secret) == "" {
		panic("RPC_INTERNAL_SECRET is required")
	}
	return func(c *clientSpec) { c.secret = secret }
}

// NewClient resolves the destination through etcd unless endpoints or a target are fixed,
// and registers the client for CloseAllClients.
func NewClient(c RpcClientConf, opts ...ClientOption) (Client, error) {
	spec := &clientSpec{conf: c}
	clientsMu.Lock()
	allClients = append(allClients, spec)
	clientsMu.Unlock()
	for _, opt := range opts {
		opt(spec)
	}
	if len(c.Etcd.Hosts) > 0 && len(c.Endpoints) == 0 && c.Target == "" {
		resolverCtx, cancel := context.WithCancel(context.Background())
		spec.cancel = cancel
		r, e := etcd.NewEtcdResolver(c.Etcd.Hosts, etcdOptions(resolverCtx, c.Etcd)...)
		if e != nil {
			cancel()
			return nil, e
		}
		spec.resolver = r
	} else if len(c.Endpoints) == 0 && c.Target == "" {
		return nil, fmt.Errorf("RPC destination is required")
	}
	return spec, nil
}

// MustNewClient panics when the client cannot be built (startup only).
func MustNewClient(c RpcClientConf, opts ...ClientOption) Client {
	v, e := NewClient(c, opts...)
	if e != nil {
		panic(e)
	}
	return v
}

// ServiceName is the registry key used for discovery and probing.
func (c *clientSpec) ServiceName() string {
	if c.conf.Etcd.Key != "" {
		return c.conf.Etcd.Key
	}
	return "direct.rpc"
}

// addresses returns fixed endpoints, or the target without its dns:/// prefix.
func (c *clientSpec) addresses() []string {
	if len(c.conf.Endpoints) > 0 {
		return c.conf.Endpoints
	}
	if c.conf.Target != "" {
		return []string{strings.TrimPrefix(c.conf.Target, "dns:///")}
	}
	return nil
}

// Options returns the Kitex client options: gRPC transport, signing, metrics, circuit breaking,
// a unary timeout and either discovery or fixed hosts.
func (c *clientSpec) Options() []client.Option {
	opts := []client.Option{client.WithTransportProtocol(transport.GRPC), client.WithConnectTimeout(time.Second), client.WithMiddleware(observed("client")), client.WithTracer(observer{side: "client"}), client.WithMetaHandler(remote.NewCustomMetaHandler(remote.WithOnConnectStream(authClient(c.secret)))), client.WithCircuitBreaker(circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key))}
	// Unary deadlines must not impose a two-second lifetime on media/SSE streams.
	timeout := c.conf.Timeout
	opts = append(opts, client.WithUnaryOptions(client.WithUnaryRPCTimeout(time.Duration(timeout)*time.Millisecond)))
	if c.resolver != nil {
		opts = append(opts, client.WithResolver(c.resolver))
	} else {
		opts = append(opts, client.WithHostPorts(c.addresses()...))
	}
	return opts
}

// Probe succeeds when any resolved instance reports SERVING to a gRPC health check within one second.
func (c *clientSpec) Probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	addresses := c.addresses()
	if c.resolver != nil {
		r, e := c.resolver.Resolve(ctx, c.ServiceName())
		if e != nil {
			return e
		}
		for _, i := range r.Instances {
			addresses = append(addresses, i.Address().String())
		}
	}
	for _, addr := range addresses {
		conn, e := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
		if e != nil {
			continue
		}
		reply, e := healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
		_ = conn.Close()
		if e == nil && reply.Status == healthpb.HealthCheckResponse_SERVING {
			return nil
		}
	}
	return fmt.Errorf("RPC dependency unavailable")
}

// ServerOptions builds the Kitex server options: signature verification, metrics, deadlines,
// admission limits, a health endpoint and etcd registration. Missing secrets or a bad address panic at startup.
func ServerOptions(c RpcServerConf, secret string) []server.Option {
	if strings.TrimSpace(secret) == "" {
		panic("RPC_INTERNAL_SECRET is required")
	}
	addr, err := net.ResolveTCPAddr("tcp", c.ListenOn)
	if err != nil {
		panic(err)
	}
	c.MustSetUp()
	opts := []server.Option{server.WithServiceAddr(addr), server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: c.Name}), server.WithMiddleware(observed("server")), server.WithMiddleware(serverDeadline(c.Timeout)), server.WithTracer(observer{side: "server"}), server.WithMetaHandler(remote.NewCustomMetaHandler(remote.WithOnReadStream(authServer(secret)))), server.WithLimit(&limit.Option{MaxConnections: c.MaxConnections, MaxQPS: c.MaxQPS}), server.WithExitWaitTime(10 * time.Second)}
	if c.Health {
		opts = append(opts, server.WithGRPCUnknownServiceHandler(func(ctx context.Context, method string, stream streaming.Stream) error {
			if method != "Check" || fullMethod(ctx) != "/grpc.health.v1.Health/Check" {
				return status.Err(codes.Unimplemented, "unknown method")
			}
			var req healthpb.HealthCheckRequest
			if err := stream.RecvMsg(&req); err != nil {
				return err
			}
			return stream.SendMsg(&healthpb.HealthCheckResponse{Status: healthpb.HealthCheckResponse_SERVING})
		}))
	}
	if len(c.Etcd.Hosts) > 0 {
		registryCtx, cancel := context.WithCancel(context.Background())
		r, e := etcd.NewEtcdRegistry(c.Etcd.Hosts, etcdOptions(registryCtx, c.Etcd)...)
		if e != nil {
			panic(e)
		}
		managed := &managedRegistry{Registry: r, cancel: cancel}
		lifecycle.TrackResource(managed)
		opts = append(opts, server.WithRegistry(managed))
		if c.Etcd.Key != "" {
			opts = append(opts, server.WithRegistryInfo(&registry.Info{ServiceName: c.Etcd.Key}))
		}
	}
	return opts
}

// NewGRPCClient is restricted to the independent Python inference trust domain.
func NewGRPCClient(c RpcClientConf) (*grpc.ClientConn, error) {
	target := c.Target
	if target == "" && len(c.Endpoints) > 0 {
		target = c.Endpoints[0]
	}
	if target == "" {
		return nil, fmt.Errorf("python RPC target is required")
	}
	return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
}

var clientsMu sync.Mutex
var allClients []*clientSpec

// Track ties a resource's lifetime to the client.
func (c *clientSpec) Track(closer io.Closer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closers = append(c.closers, closer)
}

// Close releases tracked resources and stops discovery.
func (c *clientSpec) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	for _, v := range c.closers {
		errs = append(errs, v.Close())
	}
	c.closers = nil
	if c.cancel != nil {
		c.cancel()
	}
	return errors.Join(errs...)
}

// CloseAllClients closes every client created by this process during shutdown.
func CloseAllClients() {
	clientsMu.Lock()
	clients := allClients
	allClients = nil
	clientsMu.Unlock()
	for _, c := range clients {
		_ = c.Close()
	}
}

// etcdOptions configures the registry client: credentials, key prefix, dial timeout, no proxy and silent logs.
func etcdOptions(ctx context.Context, c EtcdConf) []etcd.Option {
	return []etcd.Option{etcd.WithAuthOpt(c.User, c.Pass), etcd.WithEtcdServicePrefix(RegistryPrefix), etcd.WithDialTimeoutOpt(3 * time.Second), func(cfg *etcd.Config) {
		cfg.EtcdConfig.Context = ctx
		cfg.EtcdConfig.DialOptions = append(cfg.EtcdConfig.DialOptions, grpc.WithNoProxy())
		cfg.EtcdConfig.Logger = zap.NewNop()
	}}
}

// The pinned plugin owns its etcd client. Cancel its background context after
// deregistration, including startup failure where Deregister may never run.
type managedRegistry struct {
	registry.Registry
	cancel context.CancelFunc
}

// Deregister removes the service entry and then stops the registry client.
func (r *managedRegistry) Deregister(info *registry.Info) error {
	defer r.cancel()
	return r.Registry.Deregister(info)
}

// Close stops the registry client when deregistration never ran.
func (r *managedRegistry) Close() error { r.cancel(); return nil }

// serverDeadline caps unary work even when a caller supplies no deadline.
// Streams retain their own cancellation and lifetime policy.
func serverDeadline(timeoutMS int64) endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp any) error {
			mode := rpcinfo.GetRPCInfo(ctx).Invocation().StreamingMode()
			if timeoutMS > 0 && (mode == serviceinfo.StreamingNone || mode == serviceinfo.StreamingUnary) {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
				defer cancel()
			}
			return next(ctx, req, resp)
		}
	}
}
