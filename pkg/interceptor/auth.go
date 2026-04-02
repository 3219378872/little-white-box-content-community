package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"jwtx"

	"errx"
)

// AuthInterceptor gRPC 认证拦截器
func AuthInterceptor(config jwtx.JwtConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// 从 metadata 获取 token
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, errx.NewWithCode(errx.LOGIN_REQUIRED)
		}

		tokens := md.Get("token")
		if len(tokens) == 0 {
			return nil, errx.NewWithCode(errx.LOGIN_REQUIRED)
		}

		// 解析 token
		claims, err := jwtx.ParseToken(tokens[0], config)
		if err != nil {
			return nil, errx.NewWithCode(errx.TOKEN_INVALID)
		}

		// 将用户信息存入 context
		ctx = context.WithValue(ctx, "userId", claims.UserId)
		ctx = context.WithValue(ctx, "username", claims.Username)

		return handler(ctx, req)
	}
}

// GetUserId 从 context 获取用户 ID
func GetUserId(ctx context.Context) int64 {
	userId, _ := ctx.Value("userId").(int64)
	return userId
}

// GetUsername 从 context 获取用户名
func GetUsername(ctx context.Context) string {
	username, _ := ctx.Value("username").(string)
	return username
}
