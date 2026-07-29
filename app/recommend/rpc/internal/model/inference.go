package model

import (
	"context"
	"fmt"

	inferencepb "esx/app/recommend/rpc/xiaobaihe/inference/pb"
)

type GRPCInferenceRanker struct {
	client inferencepb.OnlineInferServiceClient
}

func NewGRPCInferenceRanker(client inferencepb.OnlineInferServiceClient) *GRPCInferenceRanker {
	return &GRPCInferenceRanker{client: client}
}

func (r *GRPCInferenceRanker) Rank(ctx context.Context, requestID, modelVersion string, candidates []PostCandidate) (InferenceResult, error) {
	requestCandidates := make([]*inferencepb.RankCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		requestCandidates = append(requestCandidates, &inferencepb.RankCandidate{
			PostId: candidate.PostID,
			Features: map[string]float64{
				"recall_score": candidate.RecallScore,
				"quality":      candidate.Features.Quality,
				"ctr":          candidate.Features.CTR,
				"freshness":    candidate.Features.Freshness,
				"popularity":   candidate.Features.Popularity,
			},
			CoarseScore: candidate.CoarseScore,
		})
	}
	response, err := r.client.Rank(ctx, &inferencepb.RankReq{
		RequestId:    requestID,
		ModelVersion: modelVersion,
		Candidates:   requestCandidates,
	})
	if err != nil {
		return InferenceResult{}, fmt.Errorf("online inference rank: %w", err)
	}
	scores := make(map[int64]float64, len(response.GetCandidates()))
	for _, candidate := range response.GetCandidates() {
		if candidate.GetPostId() <= 0 {
			return InferenceResult{}, fmt.Errorf("online inference returned invalid post id")
		}
		if _, exists := scores[candidate.GetPostId()]; exists {
			return InferenceResult{}, fmt.Errorf("online inference returned duplicate post %d", candidate.GetPostId())
		}
		scores[candidate.GetPostId()] = candidate.GetScore()
	}
	return InferenceResult{Scores: scores, ModelVersion: response.GetModelVersion()}, nil
}
