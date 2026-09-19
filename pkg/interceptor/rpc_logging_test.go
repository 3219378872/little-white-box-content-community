package interceptor

import (
	"bytes"
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	behaviorpb "esx/app/behavior/rpc/xiaobaihe/behavior/pb"
	contentpb "esx/app/content/rpc/pb/xiaobaihe/content/pb"
	feedpb "esx/app/feed/rpc/xiaobaihe/feed/pb"
	interactionpb "esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	mediapb "esx/app/media/rpc/pb/xiaobaihe/media/pb"
	messagepb "esx/app/message/rpc/xiaobaihe/message/pb"
	recommendpb "esx/app/recommend/rpc/xiaobaihe/recommend/pb"
	searchpb "esx/app/search/rpc/xiaobaihe/search/pb"
	userpb "esx/app/user/rpc/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/proc"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type lockedLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedLogBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// Use every generated method name with opaque synthetic bodies through the
// actual go-zero server/client middleware. No business services are invoked.
func TestRPCLogsExcludeBodiesAndErrorDetails(t *testing.T) {
	descriptors := []*grpc.ServiceDesc{
		&assistantpb.AssistantService_ServiceDesc, &behaviorpb.BehaviorService_ServiceDesc,
		&contentpb.ContentService_ServiceDesc, &feedpb.FeedService_ServiceDesc,
		&interactionpb.InteractionService_ServiceDesc, &mediapb.MediaService_ServiceDesc,
		&messagepb.MessageService_ServiceDesc, &recommendpb.RecommendService_ServiceDesc,
		&searchpb.SearchService_ServiceDesc, &userpb.UserService_ServiceDesc,
		{ServiceName: "future.Service", Methods: []grpc.MethodDesc{{MethodName: "NewMethod"}}},
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	var serverConf zrpc.RpcServerConf
	require.NoError(t, conf.FillDefault(&serverConf))
	serverConf.Name, serverConf.ListenOn, serverConf.Mode = "rpc-log-test", address, service.TestMode
	serverConf.DevServer.Enabled = false
	serverConf.Middlewares.StatConf.SlowThreshold = time.Millisecond
	safeConf := ServerWithoutContent(serverConf, descriptors...)
	require.True(t, safeConf.Middlewares.Stat, "keep timing metrics used by load shedding")
	require.Equal(t, serverConf.CpuThreshold, safeConf.CpuThreshold)
	ready := make(chan struct{})
	server, err := zrpc.NewServer(safeConf, func(server *grpc.Server) {
		for _, descriptor := range descriptors {
			stub := grpc.ServiceDesc{ServiceName: descriptor.ServiceName, HandlerType: (*interface{})(nil)}
			for _, method := range descriptor.Methods {
				fullMethod := "/" + descriptor.ServiceName + "/" + method.MethodName
				stub.Methods = append(stub.Methods, grpc.MethodDesc{MethodName: method.MethodName,
					Handler: func(srv any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
						request := new(wrapperspb.StringValue)
						if err := decode(request); err != nil {
							return nil, err
						}
						return interceptor(ctx, request, &grpc.UnaryServerInfo{Server: srv, FullMethod: fullMethod}, func(context.Context, any) (any, error) {
							if request.Value == "error-private-request" {
								return nil, status.Error(codes.InvalidArgument, "private-error-detail")
							}
							if request.Value == "slow-private-request" {
								time.Sleep(3 * time.Millisecond)
							}
							return wrapperspb.String("private-response"), nil
						})
					}})
			}
			server.RegisterService(&stub, struct{}{})
		}
		close(ready)
	})
	require.NoError(t, err)
	var logs lockedLogBuffer
	original := logx.Reset()
	logx.SetWriter(logx.NewWriter(&logs))
	t.Cleanup(func() { logx.SetWriter(original) })
	done := make(chan struct{})
	go func() { defer close(done); server.Start() }()
	t.Cleanup(func() {
		proc.Shutdown()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("RPC server did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatalf("RPC server did not start: %s", logs.text())
	}
	var clientConf zrpc.RpcClientConf
	require.NoError(t, conf.FillDefault(&clientConf))
	clientConf.Endpoints = []string{address}
	require.True(t, clientConf.Middlewares.Duration, "test must start with unsafe framework defaults")
	var optionCalls atomic.Int64
	client, err := NewClient(clientConf, zrpc.WithUnaryClientInterceptor(func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		optionCalls.Add(1)
		return invoker(ctx, method, req, reply, cc, opts...)
	}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Conn().Close() })
	zrpc.SetClientSlowThreshold(time.Millisecond)
	t.Cleanup(func() { zrpc.SetClientSlowThreshold(500 * time.Millisecond) })
	var calls int64
	for _, descriptor := range descriptors {
		for _, method := range descriptor.Methods {
			for _, payload := range []string{"normal-private-request", "error-private-request", "slow-private-request"} {
				var reply wrapperspb.StringValue
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := client.Conn().Invoke(ctx, "/"+descriptor.ServiceName+"/"+method.MethodName, wrapperspb.String(payload), &reply)
				cancel()
				if payload == "error-private-request" {
					require.Equal(t, codes.InvalidArgument, status.Code(err))
				} else {
					require.NoError(t, err, "server logs: %s", logs.text())
					require.Equal(t, "private-response", reply.Value)
				}
				calls++
			}
		}
	}
	require.Equal(t, calls, optionCalls.Load(), "existing client interceptors must be preserved")
	output := logs.text()
	for _, private := range []string{"private-request", "private-response", "private-error-detail"} {
		require.NotContains(t, output, private)
	}
	require.Contains(t, output, "slowcall", "exercise slow server logging")
	require.Contains(t, output, "rpc client call failed")
	require.Contains(t, output, "InvalidArgument")
	t.Logf("checked %d real RPC calls across %d service descriptors", calls, len(descriptors))
}
