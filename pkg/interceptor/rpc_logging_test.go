package interceptor

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/user"
	native "esx/kitex_gen/user/userservice"
	"esx/pkg/errx"
	"esx/pkg/lifecycle"
	"esx/pkg/logging"
	"esx/pkg/rpcx"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
func (b *lockedLogBuffer) text() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }

type loggingUserServer struct{ pb.UserService }

func (loggingUserServer) GetUser(ctx context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
	switch in.UserId {
	case 2:
		return nil, errx.NewWithCode(errx.UserNotFound)
	case 3:
		return nil, status.Error(codes.Internal, "private-error-detail")
	case 5:
		if _, ok := ctx.Deadline(); !ok {
			return nil, status.Error(codes.Internal, "missing server deadline")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	case 4:
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, Username: "private-response"}}, nil
}
func TestRPCLogsExcludeBodiesAndErrorDetails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	const secret = "rpc-transport-test-secret"
	conf := rpcx.RpcServerConf{ServiceConf: lifecycle.ServiceConf{Name: "user.rpc"}, ListenOn: addr, Timeout: 100, Health: true, MaxConnections: 100, MaxQPS: 1000}
	server := native.NewServer(&loggingUserServer{}, rpcx.ServerOptions(conf, secret)...)
	var logs lockedLogBuffer
	original := logging.Reset()
	logging.SetWriter(logging.NewWriter(&logs))
	t.Cleanup(func() { logging.SetWriter(original) })
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
	spec, err := rpcx.NewClient(rpcx.RpcClientConf{Endpoints: []string{addr}, Timeout: 1000}, rpcx.WithInternalAuth(secret))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, spec.Close()) })
	require.Eventually(t, func() bool { return spec.Probe(context.Background()) == nil }, 5*time.Second, 20*time.Millisecond)
	client := userservice.NewUserService(spec)
	for _, id := range []int64{1, 2, 3, 4} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		reply, err := client.GetUser(ctx, &pb.GetUserReq{UserId: id})
		cancel()
		switch id {
		case 2:
			var biz *errx.BizError
			require.ErrorAs(t, err, &biz)
			require.Equal(t, errx.UserNotFound, biz.Code)
		case 3:
			require.Error(t, err)
		default:
			require.NoError(t, err)
			require.Equal(t, "private-response", reply.User.Username)
		}
	}
	// A plain grpc-go client proves wire compatibility and enforces authentication.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var response pb.GetUserResp
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = conn.Invoke(ctx, "/user.UserService/GetUser", &pb.GetUserReq{UserId: 1}, &response)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	timestamp := time.Now().Unix()
	signed := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-internal-timestamp", strconv.FormatInt(timestamp, 10), "x-internal-signature", SignInternalAuthPayload(secret, timestamp, "/user.UserService/GetUser")))
	err = conn.Invoke(signed, "/user.UserService/GetUser", &pb.GetUserReq{UserId: 2}, &response)
	require.Equal(t, codes.NotFound, status.Code(err))
	require.NotEmpty(t, status.Convert(err).Details())
	// No caller deadline: the configured server cap must reach the handler.
	signed = metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-internal-timestamp", strconv.FormatInt(timestamp, 10), "x-internal-signature", SignInternalAuthPayload(secret, timestamp, "/user.UserService/GetUser")))
	finished := make(chan error, 1)
	go func() {
		var deadlineReply pb.GetUserResp
		finished <- conn.Invoke(signed, "/user.UserService/GetUser", &pb.GetUserReq{UserId: 5}, &deadlineReply)
	}()
	select {
	case err := <-finished:
		require.Equal(t, codes.DeadlineExceeded, status.Code(err))
	case <-time.After(2 * time.Second):
		t.Fatal("server deadline was ignored")
	}
	// Existing signing rules remain independently covered by internal_auth_test.go.
	for _, value := range []string{"private-response", "private-error-detail"} {
		require.NotContains(t, logs.text(), value)
	}
	require.Contains(t, logs.text(), "rpc call failed")
}
