package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type traceIDContextKey struct{}

const traceIDMetadataKey = "trace_id"

// WithTraceID 把追踪标识写入 ctx，供 TraceIDUnaryInterceptor 透传（REL-052）。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey{}, traceID)
}

// GetTraceID 从 ctx 读取追踪标识。
func GetTraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDContextKey{}).(string)
	return traceID
}

// TraceIDUnaryInterceptor 把 ctx 中的 trace_id 附加到出站 gRPC 元数据，
// 使同一请求跨能力调用保留可关联的追踪标识（REL-052）。
func TraceIDUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if traceID := GetTraceID(ctx); traceID != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, traceIDMetadataKey, traceID)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
