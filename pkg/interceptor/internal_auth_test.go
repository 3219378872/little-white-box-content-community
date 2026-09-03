package interceptor

import (
	"context"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func invokeWithMD(ctx context.Context, md metadata.MD) error {
	ctx = metadata.NewIncomingContext(ctx, md)
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	_, err := InternalAuthUnaryServerInterceptor("test-secret")(ctx, nil,
		&grpc.UnaryServerInfo{FullMethod: "/user.UserService/Login"}, handler)
	return err
}

func TestInternalAuthServerInterceptor(t *testing.T) {
	t.Run("有效签名放行", func(t *testing.T) {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		md := metadata.Pairs(
			internalTimestampMetadataKey, ts,
			internalSignatureMetadataKey, SignInternalAuthPayload("test-secret", mustAtoi(t, ts), "/user.UserService/Login"),
		)
		if err := invokeWithMD(context.Background(), md); err != nil {
			t.Fatalf("valid signature rejected: %v", err)
		}
	})

	t.Run("缺失元数据拒绝", func(t *testing.T) {
		err := invokeWithMD(context.Background(), metadata.MD{})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("missing metadata: got %v want Unauthenticated", err)
		}
	})

	t.Run("签名不符拒绝", func(t *testing.T) {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		md := metadata.Pairs(
			internalTimestampMetadataKey, ts,
			internalSignatureMetadataKey, SignInternalAuthPayload("wrong-secret", mustAtoi(t, ts), "/user.UserService/Login"),
		)
		err := invokeWithMD(context.Background(), md)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("bad signature: got %v want PermissionDenied", err)
		}
	})

	t.Run("过期时间戳拒绝", func(t *testing.T) {
		old := time.Now().Add(-10 * time.Minute).Unix()
		md := metadata.Pairs(
			internalTimestampMetadataKey, strconv.FormatInt(old, 10),
			internalSignatureMetadataKey, SignInternalAuthPayload("test-secret", old, "/user.UserService/Login"),
		)
		err := invokeWithMD(context.Background(), md)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("stale timestamp: got %v want Unauthenticated", err)
		}
	})

	t.Run("时间戳格式非法拒绝", func(t *testing.T) {
		md := metadata.Pairs(
			internalTimestampMetadataKey, "not-a-number",
			internalSignatureMetadataKey, "whatever",
		)
		err := invokeWithMD(context.Background(), md)
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("malformed timestamp: got %v want Unauthenticated", err)
		}
	})

	t.Run("签名绑定方法，不能跨 RPC 重放", func(t *testing.T) {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		md := metadata.Pairs(
			internalTimestampMetadataKey, ts,
			internalSignatureMetadataKey, SignInternalAuthPayload("test-secret", mustAtoi(t, ts), "/user.UserService/Register"),
		)
		err := invokeWithMD(context.Background(), md)
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("cross-method replay: got %v want PermissionDenied", err)
		}
	})

	t.Run("健康检查豁免", func(t *testing.T) {
		handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
		_, err := InternalAuthUnaryServerInterceptor("test-secret")(context.Background(), nil,
			&grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, handler)
		if err != nil {
			t.Fatalf("health check must be exempt: %v", err)
		}
	})
}

func TestInternalAuthClientInterceptorSignsOutgoing(t *testing.T) {
	var captured metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		captured, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
	inter := InternalAuthUnaryClientInterceptor("test-secret")
	err := inter(context.Background(), "/user.UserService/Login", nil, nil, nil, invoker)
	if err != nil {
		t.Fatal(err)
	}
	ts := captured.Get(internalTimestampMetadataKey)
	sigs := captured.Get(internalSignatureMetadataKey)
	if len(ts) != 1 || len(sigs) != 1 {
		t.Fatalf("outgoing metadata missing credentials: %v", captured)
	}
	unix, err := strconv.ParseInt(ts[0], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if sigs[0] != SignInternalAuthPayload("test-secret", unix, "/user.UserService/Login") {
		t.Fatal("signature does not match expected HMAC")
	}
}

type internalAuthTestServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *internalAuthTestServerStream) Context() context.Context { return s.ctx }

func TestInternalAuthStreamServerInterceptor(t *testing.T) {
	ts := time.Now().Unix()
	valid := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		internalTimestampMetadataKey, strconv.FormatInt(ts, 10),
		internalSignatureMetadataKey, SignInternalAuthPayload("test-secret", ts, "/media.MediaService/UploadImage"),
	))
	called := false
	handler := func(any, grpc.ServerStream) error {
		called = true
		return nil
	}
	inter := InternalAuthStreamServerInterceptor("test-secret")
	if err := inter(nil, &internalAuthTestServerStream{ctx: valid},
		&grpc.StreamServerInfo{FullMethod: "/media.MediaService/UploadImage"}, handler); err != nil {
		t.Fatalf("valid stream signature rejected: %v", err)
	}
	if !called {
		t.Fatal("valid stream did not reach handler")
	}

	called = false
	err := inter(nil, &internalAuthTestServerStream{ctx: context.Background()},
		&grpc.StreamServerInfo{FullMethod: "/media.MediaService/UploadImage"}, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unsigned stream: got %v want Unauthenticated", err)
	}
	if called {
		t.Fatal("unsigned stream reached handler")
	}
}

func TestInternalAuthStreamClientInterceptorSignsOutgoing(t *testing.T) {
	var captured metadata.MD
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn,
		_ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		captured, _ = metadata.FromOutgoingContext(ctx)
		return nil, nil
	}
	_, err := InternalAuthStreamClientInterceptor("test-secret")(
		context.Background(), &grpc.StreamDesc{}, nil, "/assistant.AssistantService/SubscribeRunEvents", streamer)
	if err != nil {
		t.Fatal(err)
	}
	ts := captured.Get(internalTimestampMetadataKey)
	sigs := captured.Get(internalSignatureMetadataKey)
	if len(ts) != 1 || len(sigs) != 1 {
		t.Fatalf("outgoing stream metadata missing credentials: %v", captured)
	}
	unix, err := strconv.ParseInt(ts[0], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if sigs[0] != SignInternalAuthPayload("test-secret", unix, "/assistant.AssistantService/SubscribeRunEvents") {
		t.Fatal("stream signature does not match expected HMAC")
	}
}

func TestInternalAuthEmptySecretPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("empty secret must fail fast")
		}
	}()
	InternalAuthUnaryServerInterceptor("  ")
}

func mustAtoi(t *testing.T, s string) int64 {
	t.Helper()
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
