package rpcx_test

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	assistantclient "esx/app/assistant/rpc/assistantservice"
	mediaclient "esx/app/media/rpc/mediaservice"
	assistantpb "esx/kitex_gen/assistant"
	assistantserver "esx/kitex_gen/assistant/assistantservice"
	mediapb "esx/kitex_gen/media"
	mediaserver "esx/kitex_gen/media/mediaservice"
	"esx/pkg/errx"
	"esx/pkg/lifecycle"
	"esx/pkg/rpcx"

	"github.com/cloudwego/kitex/pkg/streaming"
	"github.com/cloudwego/kitex/server"
	"github.com/stretchr/testify/require"
)

type assistantTransport struct {
	assistantpb.AssistantService
	canceled chan struct{}
}

func (s *assistantTransport) SubscribeRunEvents(req *assistantpb.SubscribeRunEventsReq, stream assistantpb.AssistantService_SubscribeRunEventsServer) error {
	if req.RunId == 2 {
		return errx.NewWithCode(errx.PermissionDenied)
	}
	if rpcx.TraceID(stream.Context()) != "stream-trace" {
		return errx.NewWithCode(errx.ParamError)
	}
	if err := stream.Send(&assistantpb.RunEvent{RunId: req.RunId, Seq: 1, Text: "first"}); err != nil {
		return err
	}
	if req.RunId == 3 {
		<-stream.Context().Done()
		close(s.canceled)
		return stream.Context().Err()
	}
	select {
	case <-time.After(150 * time.Millisecond):
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	return stream.Send(&assistantpb.RunEvent{RunId: req.RunId, Seq: 2, Text: "last"})
}

type mediaTransport struct{ mediapb.MediaService }

func (s *mediaTransport) UploadImage(stream mediapb.MediaService_UploadImageServer) error {
	count := int64(0)
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return errx.NewWithCode(errx.ParamError)
	}
	return stream.SendAndClose(&mediapb.UploadImageResp{Media: &mediapb.MediaInfo{Id: count}})
}
func startTransport(t *testing.T, create func(...server.Option) server.Server) rpcx.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	conf := rpcx.RpcServerConf{ServiceConf: lifecycle.ServiceConf{Name: "stream.rpc"}, ListenOn: addr, Health: true, MaxConnections: 100, MaxQPS: 1000}
	srv := create(rpcx.ServerOptions(conf, "stream-secret")...)
	done := make(chan error, 1)
	go func() { done <- srv.Run() }()
	t.Cleanup(func() {
		require.NoError(t, srv.Stop())
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("server drain timed out")
		}
	})
	spec, err := rpcx.NewClient(rpcx.RpcClientConf{Endpoints: []string{addr}, Timeout: 50}, rpcx.WithInternalAuth("stream-secret"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, spec.Close()) })
	require.Eventually(t, func() bool { return spec.Probe(context.Background()) == nil }, 3*time.Second, 10*time.Millisecond)
	return spec
}
func TestServerStreamTraceErrorsCancellationAndLifetime(t *testing.T) {
	impl := &assistantTransport{canceled: make(chan struct{})}
	spec := startTransport(t, func(opts ...server.Option) server.Server { return assistantserver.NewServer(impl, opts...) })
	client := assistantclient.NewAssistantService(spec)
	ctx, cancel := context.WithTimeout(rpcx.WithTraceID(context.Background(), "stream-trace"), 3*time.Second)
	defer cancel()
	stream, err := client.SubscribeRunEvents(ctx, &assistantpb.SubscribeRunEventsReq{RunId: 1})
	require.NoError(t, err)
	first, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Seq)
	last, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, int64(2), last.Seq)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
	streaming.FinishStream(stream, err)
	denied, err := client.SubscribeRunEvents(ctx, &assistantpb.SubscribeRunEventsReq{RunId: 2})
	require.NoError(t, err)
	_, err = denied.Recv()
	var biz *errx.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errx.PermissionDenied, biz.Code)
	subscription, cancelSubscription := context.WithCancel(ctx)
	active, err := client.SubscribeRunEvents(subscription, &assistantpb.SubscribeRunEventsReq{RunId: 3})
	require.NoError(t, err)
	_, err = active.Recv()
	require.NoError(t, err)
	cancelSubscription()
	_, err = active.Recv()
	require.Error(t, err)
	select {
	case <-impl.canceled:
	case <-time.After(time.Second):
		t.Fatal("server subscription did not observe cancellation")
	}
}
func TestClientStreamHalfCloseAndBusinessError(t *testing.T) {
	spec := startTransport(t, func(opts ...server.Option) server.Server { return mediaserver.NewServer(&mediaTransport{}, opts...) })
	client := mediaclient.NewMediaService(spec)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	upload, err := client.UploadImage(ctx)
	require.NoError(t, err)
	for range 4 {
		require.NoError(t, upload.Send(&mediapb.UploadImageReq{}))
	}
	response, err := upload.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, int64(4), response.Media.Id)
	streaming.FinishStream(upload, err)
	empty, err := client.UploadImage(ctx)
	require.NoError(t, err)
	_, err = empty.CloseAndRecv()
	var biz *errx.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errx.ParamError, biz.Code)
}
