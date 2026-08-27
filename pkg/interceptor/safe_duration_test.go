package interceptor

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
)

func TestSafeDurationUnaryClientInterceptorPreservesResult(t *testing.T) {
	want := errors.New("rpc failed")
	interceptor := SafeDurationUnaryClientInterceptor()
	err := interceptor(context.Background(), "/svc.Method/Call", struct{ Secret string }{"hidden"}, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return want
		})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}
