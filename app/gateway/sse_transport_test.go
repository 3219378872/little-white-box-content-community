package main

import (
	"bufio"
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"esx/app/assistant/rpc/assistantservice"
	pb "esx/kitex_gen/assistant"
	"esx/pkg/jwtx"

	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/streaming"
	"github.com/stretchr/testify/require"
)

type socketAssistant struct {
	contractAssistantService
	requests chan int64
	stopped  chan struct{}
	canceled atomic.Bool
}

func (s *socketAssistant) SubscribeRunEvents(ctx context.Context, req *pb.SubscribeRunEventsReq, _ ...callopt.Option) (assistantservice.AssistantService_SubscribeRunEventsClient, error) {
	s.requests <- req.AfterSeq
	return &socketSubscription{ctx: ctx, stopped: s.stopped}, nil
}
func (s *socketAssistant) CancelRun(context.Context, *pb.CancelRunReq, ...callopt.Option) (*pb.CancelRunResp, error) {
	s.canceled.Store(true)
	return &pb.CancelRunResp{}, nil
}

type socketSubscription struct {
	streaming.Stream
	ctx     context.Context
	stopped chan struct{}
	sent    bool
}

func (s *socketSubscription) Recv() (*pb.RunEvent, error) {
	if !s.sent {
		s.sent = true
		return &pb.RunEvent{RunId: 9, Seq: 10, Type: "token", Text: "first"}, nil
	}
	<-s.ctx.Done()
	close(s.stopped)
	return nil, s.ctx.Err()
}
func TestSSEFlushReplayCursorAndDisconnectDoNotCancelRun(t *testing.T) {
	service := &socketAssistant{requests: make(chan int64, 1), stopped: make(chan struct{})}
	base, _ := startContractServerWithAssistant(t, service)
	token, err := jwtx.GenerateToken(1, "alice", jwtx.JwtConfig{AccessSecret: contractSecret, AccessExpire: 3600})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/api/v2/assistant/runs/9/events?afterSeq=7", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Last-Event-ID", "9")
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport}).Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	// Headers and a complete frame arrive while the upstream Recv is still blocked.
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "id: 10\n", line)
	require.Equal(t, int64(9), <-service.requests)
	require.NoError(t, response.Body.Close())
	select {
	case <-service.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP disconnect did not cancel upstream subscription")
	}
	require.False(t, service.canceled.Load(), "disconnect must not cancel the persistent run")
}
