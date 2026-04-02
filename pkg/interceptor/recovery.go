package interceptor

import (
	"context"
	"runtime/debug"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"errx"
)

// RecoveryInterceptor 异常恢复拦截器
func RecoveryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				// 记录堆栈信息
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("method", info.FullMethod),
					zap.String("stack", string(debug.Stack())),
				)

				// 返回系统错误
				err = status.Errorf(codes.Internal, "internal error: %v", r)
			}
		}()

		return handler(ctx, req)
	}
}

// RecoveryWithErrxInterceptor 带 errx 的异常恢复拦截器
func RecoveryWithErrxInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("method", info.FullMethod),
					zap.String("stack", string(debug.Stack())),
				)
				err = errx.NewWithCode(errx.SYSTEM_ERROR)
			}
		}()

		return handler(ctx, req)
	}
}
