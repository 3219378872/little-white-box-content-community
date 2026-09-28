//go:build integration

package rpcx_test

import (
	"context"
	"net"
	"testing"
	"time"

	"esx/app/user/rpc/userservice"
	pb "esx/kitex_gen/user"
	native "esx/kitex_gen/user/userservice"
	"esx/pkg/lifecycle"
	"esx/pkg/rpcx"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type discoveredUser struct {
	pb.UserService
	version string
}

func (s *discoveredUser) GetUser(_ context.Context, in *pb.GetUserReq) (*pb.GetUserResp, error) {
	return &pb.GetUserResp{User: &pb.UserInfo{Id: in.UserId, Username: s.version}}, nil
}
func TestEtcdDiscoveryRecoversAfterInstanceReplacement(t *testing.T) {
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{Image: "quay.io/coreos/etcd:v3.5.5", ExposedPorts: []string{"2379/tcp"}, Cmd: []string{"etcd", "--listen-client-urls=http://0.0.0.0:2379", "--advertise-client-urls=http://127.0.0.1:2379"}, WaitingFor: wait.ForListeningPort("2379/tcp"), Started: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(container)) })
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "2379/tcp")
	require.NoError(t, err)
	etcd := rpcx.EtcdConf{Hosts: []string{net.JoinHostPort(host, port.Port())}, Key: "migration.user.rpc"}
	spec, err := rpcx.NewClient(rpcx.RpcClientConf{Etcd: etcd, Timeout: 500}, rpcx.WithInternalAuth("discovery-secret"))
	require.NoError(t, err)
	defer spec.Close()
	start := func(version string) func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())
		conf := rpcx.RpcServerConf{ServiceConf: lifecycle.ServiceConf{Name: "display.user.rpc"}, ListenOn: addr, Etcd: etcd, Health: true, MaxConnections: 100, MaxQPS: 1000}
		server := native.NewServer(&discoveredUser{version: version}, rpcx.ServerOptions(conf, "discovery-secret")...)
		done := make(chan error, 1)
		go func() { done <- server.Run() }()
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			require.NoError(t, server.Stop())
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("server did not drain")
			}
		}
		t.Cleanup(stop)
		require.Eventually(t, func() bool { return spec.Probe(ctx) == nil }, 5*time.Second, 30*time.Millisecond)
		return stop
	}
	stopFirst := start("first")
	client := userservice.NewUserService(spec)
	first, err := client.GetUser(ctx, &pb.GetUserReq{UserId: 7})
	require.NoError(t, err)
	require.Equal(t, "first", first.User.Username)
	stopFirst()
	require.Eventually(t, func() bool { return spec.Probe(ctx) != nil }, 5*time.Second, 30*time.Millisecond)
	start("replacement")
	require.Eventually(t, func() bool {
		reply, err := client.GetUser(ctx, &pb.GetUserReq{UserId: 7})
		return err == nil && reply.User.Username == "replacement"
	}, 20*time.Second, 200*time.Millisecond)
}
