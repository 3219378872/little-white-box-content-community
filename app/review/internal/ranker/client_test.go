package ranker

import (
	"context"
	"errors"
	"testing"

	"esx/app/review/internal/cascade"
	pb "esx/app/review/xiaobaihe/moderation/pb"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeClient struct {
	resp *pb.ScoreResp
	err  error
	req  *pb.ScoreReq
}

func (f *fakeClient) Score(_ context.Context, in *pb.ScoreReq, _ ...grpc.CallOption) (*pb.ScoreResp, error) {
	f.req = in
	return f.resp, f.err
}

func (f *fakeClient) Health(context.Context, *pb.ModerationHealthReq, ...grpc.CallOption) (*pb.ModerationHealthResp, error) {
	return &pb.ModerationHealthResp{Ready: true}, nil
}

func TestScoreMapsResponseAndErrors(t *testing.T) {
	fake := &fakeClient{resp: &pb.ScoreResp{ModelVersion: "stub-v0", Scores: []*pb.IssueScore{{Issue: "A.B", PYes: 0.5, PNo: 0.5}}}}
	client := NewWithClient(fake)
	result, err := client.Score(context.Background(), cascade.RankInput{TaskID: 3, Text: "t", Issues: []string{"A.B"}})
	require.NoError(t, err)
	require.Equal(t, "stub-v0", result.ModelVersion)
	require.Equal(t, int64(3), fake.req.TaskId)
	require.Len(t, result.Scores, 1)

	fake.err = status.Error(codes.DeadlineExceeded, "slow")
	_, err = client.Score(context.Background(), cascade.RankInput{})
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	fake.err = status.Error(codes.Unavailable, "down")
	_, err = client.Score(context.Background(), cascade.RankInput{})
	require.True(t, errors.Is(err, cascade.ErrUnavailable))
	require.NoError(t, client.Close())
}
