package interceptor

import (
	"context"
	"errx"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestBizErrorInterceptor_NilError(t *testing.T) {
	interceptor := BizErrorUnaryInterceptor()
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return nil
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestBizErrorInterceptor_ConvertsBizError(t *testing.T) {
	interceptor := BizErrorUnaryInterceptor()

	// Simulate: server returned BizError{1001, "用户不存在"} via GRPCStatus()
	st := status.New(codes.NotFound, "用户不存在")
	detailed, _ := st.WithDetails(wrapperspb.Int32(int32(errx.UserNotFound)))
	rpcErr := detailed.Err()

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return rpcErr
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)

	bizErr, ok := err.(*errx.BizError)
	if !ok {
		t.Fatalf("expected *errx.BizError, got %T", err)
	}
	if bizErr.Code != errx.UserNotFound {
		t.Errorf("Code = %d, want %d", bizErr.Code, errx.UserNotFound)
	}
	if bizErr.Message != "用户不存在" {
		t.Errorf("Message = %q, want %q", bizErr.Message, "用户不存在")
	}
}

func TestBizErrorInterceptor_FrameworkError(t *testing.T) {
	interceptor := BizErrorUnaryInterceptor()

	// Framework-generated error (no BizError detail)
	rpcErr := status.Error(codes.DeadlineExceeded, "context deadline exceeded")

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return rpcErr
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)

	bizErr, ok := err.(*errx.BizError)
	if !ok {
		t.Fatalf("expected *errx.BizError, got %T", err)
	}
	if bizErr.Code != errx.SystemError {
		t.Errorf("Code = %d, want %d", bizErr.Code, errx.SystemError)
	}
}
