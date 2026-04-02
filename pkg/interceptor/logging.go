package interceptor

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor 日志拦截器
func LoggingInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// 获取 trace ID
		spanCtx := trace.SpanContextFromContext(ctx)
		traceID := spanCtx.TraceID().String()

		// 处理请求
		resp, err := handler(ctx, req)

		// 记录日志
		duration := time.Since(start)
		code := status.Code(err)

		if err != nil {
			logger.Error("gRPC call",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
				zap.String("traceID", traceID),
				zap.String("code", code.String()),
				zap.Error(err),
			)
		} else {
			logger.Info("gRPC call",
				zap.String("method", info.FullMethod),
				zap.Duration("duration", duration),
				zap.String("traceID", traceID),
				zap.String("code", code.String()),
			)
		}

		return resp, err
	}
}
