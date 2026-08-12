package interceptor

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestTraceIDUnaryInterceptorAttachesMetadata(t *testing.T) {
	ctx := WithTraceID(context.Background(), "trace-123")
	var gotTraceID string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("no outgoing metadata")
		}
		values := md.Get("trace_id")
		if len(values) != 1 {
			t.Fatalf("trace_id values = %v", values)
		}
		gotTraceID = values[0]
		return nil
	}
	err := TraceIDUnaryInterceptor()(ctx, "/svc/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatal(err)
	}
	if gotTraceID != "trace-123" {
		t.Fatalf("trace_id = %q, want trace-123", gotTraceID)
	}
}

func TestTraceIDUnaryInterceptorSkipsWithoutTrace(t *testing.T) {
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		if md, ok := metadata.FromOutgoingContext(ctx); ok {
			if values := md.Get("trace_id"); len(values) > 0 {
				t.Fatalf("unexpected trace_id values: %v", values)
			}
		}
		return nil
	}
	if err := TraceIDUnaryInterceptor()(context.Background(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatal(err)
	}
}
