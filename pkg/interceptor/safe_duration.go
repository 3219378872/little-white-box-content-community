package interceptor

import (
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc"
)

// SafeDurationUnaryClientInterceptor replaces go-zero's duration interceptor,
// which includes the complete protobuf request when an RPC fails.
func SafeDurationUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		started := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			logx.WithContext(ctx).Errorw("rpc client call failed",
				logx.Field("method", method),
				logx.Field("duration_ms", time.Since(started).Milliseconds()),
				logx.Field("err", err.Error()))
		}
		return err
	}
}
