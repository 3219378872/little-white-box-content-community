package svc

import (
	"context"
	"errors"
	native "esx/kitex_gen/content/contentservice"
	"esx/pkg/lifecycle"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"esx/app/content/rpc/contentservice"
	contentpb "esx/kitex_gen/content"
	"esx/pkg/errx"

	"esx/pkg/rpcx"
)

const internalAuthTestSecret = "svc-internal-auth-test-secret"

type stubContentServer struct {
	contentpb.ContentService
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
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	server := native.NewServer(&stubContentServer{}, rpcx.ServerOptions(rpcx.RpcServerConf{
		ServiceConf: lifecycle.ServiceConf{Name: "content.rpc"}, ListenOn: addr, Health: true,
		MaxConnections: 100, MaxQPS: 1000,
	}, internalAuthTestSecret)...)
	done := make(chan error, 1)
	go func() { done <- server.Run() }()
	t.Cleanup(func() {
		require.NoError(t, server.Stop())
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Kitex server did not stop")
		}
	})
	newClient := func(opts ...rpcx.ClientOption) contentservice.ContentService {
		t.Helper()
		client, err := rpcx.NewClient(rpcx.RpcClientConf{Endpoints: []string{addr}, Timeout: 3000}, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, client.Close()) })
		require.Eventually(t, func() bool { return client.Probe(context.Background()) == nil }, 5*time.Second, 20*time.Millisecond)
		return contentservice.NewContentService(client)
	}
	signed := newClient(rpcx.WithInternalAuth(internalAuthTestSecret))
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
