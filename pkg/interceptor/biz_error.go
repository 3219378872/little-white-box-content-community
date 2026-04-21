package interceptor

import (
	"context"
	"errx"

	"google.golang.org/grpc"
)

// BizErrorUnaryInterceptor returns a gRPC client unary interceptor that converts
// gRPC status errors back to errx.BizError using errx.FromGRPCError.
func BizErrorUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			return errx.FromGRPCError(err)
		}
		return nil
	}
}
