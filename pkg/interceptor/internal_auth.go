package interceptor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// 服务间内部鉴权（纵深防御）：RPC 端口只暴露在内网，但内网失守
// （被攻陷容器/SSRF/误接线）时不应能冒充任意用户调用写接口。
// 客户端对时间戳做 HMAC-SHA256 签名，服务端校验签名与时间窗口，
// 使外部实体无法仅凭网络位置伪造合法调用方。
//
// 约定：
//   - secret 通过环境变量注入（RPC_INTERNAL_SECRET），全服务共享；
//   - 健康检查与 reflection 不签名（探针与开发工具不持有 secret）；
//   - 跨语言旁路（recommend→online-infer、embedding→embedding-service）
//     属独立信任域，不挂本拦截器。

const (
	internalTimestampMetadataKey = "x-internal-timestamp"
	internalSignatureMetadataKey = "x-internal-signature"

	// internalAuthMaxClockSkew 允许的客户端与服务端时钟偏差。
	internalAuthMaxClockSkew = 5 * time.Minute
)

func requireInternalSecret(secret string) {
	if strings.TrimSpace(secret) == "" {
		panic("interceptor: internal auth secret is empty; set RPC_INTERNAL_SECRET")
	}
}

func internalSignature(secret, timestamp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	return hex.EncodeToString(mac.Sum(nil))
}

// internalAuthExemptMethod 判断方法是否豁免内部鉴权。
func internalAuthExemptMethod(method string) bool {
	return strings.HasPrefix(method, "/grpc.health.v1.Health/") ||
		strings.HasPrefix(method, "/grpc.reflection.")
}

// InternalAuthUnaryServerInterceptor 校验入站请求的内部签名；
// 缺失、过期或签名不符一律拒绝。secret 为空时启动即失败。
func InternalAuthUnaryServerInterceptor(secret string) grpc.UnaryServerInterceptor {
	requireInternalSecret(secret)

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (any, error) {
		if internalAuthExemptMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "internal auth metadata missing")
		}
		timestamps := md.Get(internalTimestampMetadataKey)
		signatures := md.Get(internalSignatureMetadataKey)
		if len(timestamps) != 1 || len(signatures) != 1 {
			return nil, status.Error(codes.Unauthenticated, "internal auth credentials missing")
		}

		ts, err := strconv.ParseInt(timestamps[0], 10, 64)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "internal auth timestamp malformed")
		}
		if skew := time.Since(time.Unix(ts, 0)); skew > internalAuthMaxClockSkew || skew < -internalAuthMaxClockSkew {
			return nil, status.Error(codes.Unauthenticated, "internal auth timestamp expired")
		}

		if !hmac.Equal([]byte(internalSignature(secret, timestamps[0])), []byte(signatures[0])) {
			return nil, status.Error(codes.PermissionDenied, "internal auth signature mismatch")
		}
		return handler(ctx, req)
	}
}

// InternalAuthUnaryClientInterceptor 为出站请求附加时间戳与 HMAC 签名。
// secret 为空时启动即失败。
func InternalAuthUnaryClientInterceptor(secret string) grpc.UnaryClientInterceptor {
	requireInternalSecret(secret)

	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if internalAuthExemptMethod(method) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		ctx = metadata.AppendToOutgoingContext(ctx,
			internalTimestampMetadataKey, timestamp,
			internalSignatureMetadataKey, internalSignature(secret, timestamp),
		)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// SignInternalAuthPayload 供测试与非 gRPC 场景复用的签名计算。
func SignInternalAuthPayload(secret string, unixSeconds int64) string {
	if strings.TrimSpace(secret) == "" {
		panic(fmt.Sprintf("interceptor: %s", "internal auth secret is empty"))
	}
	return internalSignature(secret, strconv.FormatInt(unixSeconds, 10))
}
