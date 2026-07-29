package logic

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/model"
	inferencepb "esx/app/recommend/rpc/xiaobaihe/inference/pb"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type faultingOnlineInferServer struct {
	inferencepb.UnimplementedOnlineInferServiceServer
	mode string
}

func (s faultingOnlineInferServer) Rank(ctx context.Context, _ *inferencepb.RankReq) (*inferencepb.RankResp, error) {
	switch s.mode {
	case "timeout":
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	default:
		return nil, status.Error(codes.Unavailable, "injected online inference outage")
	}
}

func newFaultingInferenceRanker(t *testing.T, mode string) model.InferenceRanker {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for faulting inference server: %v", err)
	}
	server := grpc.NewServer()
	inferencepb.RegisterOnlineInferServiceServer(server, faultingOnlineInferServer{mode: mode})
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	connection, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		server.Stop()
		listener.Close()
		t.Fatalf("connect to faulting inference server: %v", err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		server.Stop()
		_ = listener.Close()
		<-serveResult
	})
	return model.NewGRPCInferenceRanker(inferencepb.NewOnlineInferServiceClient(connection))
}

func TestGetRecommendPostsFaultInjectionOnlineInfer(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		timeout         time.Duration
		wantDegradation string
	}{
		{name: "grpc unavailable", mode: "unavailable", timeout: 200 * time.Millisecond, wantDegradation: "infer-unavailable"},
		{name: "grpc deadline", mode: "timeout", timeout: 20 * time.Millisecond, wantDegradation: "infer-timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
			serviceContext.Config.OnlineInfer = config.OnlineInferConfig{
				Enabled: true, ModelVersion: "rank-production", TimeoutMs: tt.timeout.Milliseconds(),
			}
			serviceContext.Config.ExploreRatio = 0
			serviceContext.PostRecallSources = []model.PostRecallSource{
				fakePostSource{name: "hot", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
					return []model.PostCandidate{
						knownPost(11, 101, "tech", 0.9),
						knownPost(12, 102, "culture", 0.1),
					}, nil
				}},
			}
			serviceContext.InferenceRanker = newFaultingInferenceRanker(t, tt.mode)

			started := time.Now()
			response, err := NewGetRecommendPostsLogic(context.Background(), serviceContext).GetRecommendPosts(
				&pb.GetRecommendPostsReq{UserId: 7, RequestId: "fault-injection", PageSize: 2},
			)
			elapsed := time.Since(started)

			if err != nil {
				t.Fatalf("GetRecommendPosts() returned error during injected fault: %v", err)
			}
			if len(response.Posts) != 2 || response.Posts[0].PostId != 11 || response.Posts[1].PostId != 12 {
				t.Fatalf("rule fallback did not preserve a usable ranking: %+v", response.Posts)
			}
			for _, post := range response.Posts {
				if !strings.Contains(post.ModelVersion, tt.wantDegradation) || !strings.Contains(post.ModelVersion, "rules-test") {
					t.Fatalf("model version %q does not expose rule degradation %q", post.ModelVersion, tt.wantDegradation)
				}
			}
			if elapsed >= time.Second {
				t.Fatalf("fault fallback took %s; expected bounded completion", elapsed)
			}
		})
	}
}
