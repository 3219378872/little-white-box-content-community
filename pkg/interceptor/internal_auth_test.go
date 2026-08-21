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
			internalSignatureMetadataKey, SignInternalAuthPayload("test-secret", mustAtoi(t, ts)),
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
			internalSignatureMetadataKey, SignInternalAuthPayload("wrong-secret", mustAtoi(t, ts)),
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
			internalSignatureMetadataKey, SignInternalAuthPayload("test-secret", old),
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
	if sigs[0] != SignInternalAuthPayload("test-secret", unix) {
		t.Fatal("signature does not match expected HMAC")
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
