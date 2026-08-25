package svc

import (
	"context"
	"errors"
	"net"
	"testing"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	"esx/pkg/errx"
	"esx/pkg/interceptor"

	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

const internalAuthTestSecret = "svc-internal-auth-test-secret"

type stubContentServer struct {
	contentpb.UnimplementedContentServiceServer
}

func (s *stubContentServer) GetPostList(ctx context.Context,
	in *contentpb.GetPostListReq,
) (*contentpb.GetPostListResp, error) {
	return &contentpb.GetPostListResp{Posts: []*contentpb.PostInfo{}}, nil
}

// 启动一个带内部签名校验的 content 服务，验证 recommend 侧出站拦截器的
// 组合行为：签名客户端可调用；缺失签名时服务端 Unauthenticated 被
// errx.FromGRPCError 映射为 LoginRequired（1006），即推荐降级的根因。
func TestInternalAuthClientInterceptorWiring(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.InternalAuthUnaryServerInterceptor(internalAuthTestSecret),
		),
	)
	contentpb.RegisterContentServiceServer(server, &stubContentServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	newClient := func(interceptors ...zrpc.ClientOption) contentservice.ContentService {
		t.Helper()
		opts := append([]zrpc.ClientOption{
			zrpc.WithUnaryClientInterceptor(interceptor.BizErrorUnaryInterceptor()),
		}, interceptors...)
		client, err := zrpc.NewClient(zrpc.RpcClientConf{
			Endpoints: []string{listener.Addr().String()},
			Timeout:   3000,
		}, opts...)
		if err != nil {
			t.Fatalf("new zrpc client: %v", err)
		}
		t.Cleanup(func() { _ = client.Conn().Close() })
		return contentservice.NewContentService(client)
	}

	signed := newClient(
		zrpc.WithUnaryClientInterceptor(
			interceptor.InternalAuthUnaryClientInterceptor(internalAuthTestSecret)),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5_000_000_000)
	defer cancel()
	if _, err := signed.GetPostList(ctx, &contentpb.GetPostListReq{PageSize: 5}); err != nil {
		t.Fatalf("signed internal call must succeed, got %v", err)
	}

	unsigned := newClient()
	_, err = unsigned.GetPostList(ctx, &contentpb.GetPostListReq{PageSize: 5})
	if err == nil {
		t.Fatal("unsigned internal call must be rejected by the server interceptor")
	}
	var bizErr *errx.BizError
	if !errors.As(err, &bizErr) || bizErr.Code != errx.LoginRequired {
		t.Fatalf("unsigned call should surface as errx %d (LoginRequired), got %v",
			errx.LoginRequired, err)
	}
}
