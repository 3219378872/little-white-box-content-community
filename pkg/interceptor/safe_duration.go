package interceptor

import (
	"context"
	"time"

	"esx/pkg/logging"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// SafeDurationUnaryClientInterceptor records standard gRPC timing without payloads,
// which includes the complete protobuf request when an RPC fails.
func SafeDurationUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		started := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			logging.WithContext(ctx).Errorw("rpc client call failed",
				logging.Field("method", method),
				logging.Field("duration_ms", time.Since(started).Milliseconds()),
				logging.Field("grpc_code", status.Code(err).String()))
		}
		return err
	}
}
