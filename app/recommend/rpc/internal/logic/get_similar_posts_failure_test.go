package logic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"
	"esx/pkg/errx"
)

func similarRequest() *pb.GetSimilarPostsReq {
	return &pb.GetSimilarPostsReq{
		PostId: 10, Limit: 5, RequestId: "similar-fail", ExperimentId: "exp-1",
	}
}

func TestGetSimilarPostsRejectsInvalidParams(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	logic := NewGetSimilarPostsLogic(context.Background(), serviceContext)

	for name, req := range map[string]*pb.GetSimilarPostsReq{
		"nil request":  nil,
		"zero postId":  {RequestId: "r1"},
		"no requestId": {PostId: 10},
		"long scene":   {PostId: 10, RequestId: "r1", Scene: string(make([]byte, 65))},
		"long exp id":  {PostId: 10, RequestId: "r1", ExperimentId: string(make([]byte, 129))},
	} {
		_, err := logic.GetSimilarPosts(req)
		if !errx.Is(err, errx.ParamError) {
			t.Fatalf("%s: error = %v, want ParamError", name, err)
		}
	}
}

func TestGetSimilarPostsRejectsOversizedLimit(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	_, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).
		GetSimilarPosts(&pb.GetSimilarPostsReq{PostId: 10, Limit: 999, RequestId: "r1"})
	if !errx.Is(err, errx.ParamError) {
		t.Fatalf("error = %v, want ParamError", err)
	}
}

func TestGetSimilarPostsRecallUnavailableWhenAllSourcesFail(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.SimilarPostSources = []model.PostRecallSource{
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, errors.New("recall store down")
		}},
	}
	_, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).GetSimilarPosts(similarRequest())
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("error = %v, want ServiceUnavailable", err)
	}
}

func TestGetSimilarPostsFailsWhenDegradedRecallYieldsNoCandidates(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.SimilarPostSources = []model.PostRecallSource{
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, errors.New("recall store down")
		}},
		fakePostSource{name: "milvus", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, nil
		}},
	}
	_, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).GetSimilarPosts(similarRequest())
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("error = %v, want ServiceUnavailable", err)
	}
}

func TestGetSimilarPostsVisibilityCheckFailureIsServiceUnavailable(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.SimilarPostSources = []model.PostRecallSource{
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(11, 2, "tech", 0.8)}, nil
		}},
	}
	content := &fakeRecommendContentService{err: errors.New("content rpc down")}
	serviceContext.ContentService = content
	_, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).GetSimilarPosts(similarRequest())
	if !errx.Is(err, errx.ServiceUnavailable) {
		t.Fatalf("error = %v, want ServiceUnavailable", err)
	}
}

func TestGetSimilarPostsMarksRecallAndFeatureDegradation(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	// 一个源失败（部分降级）+ FeatureRepository 缺失（特征降级），候选数小于 limit。
	serviceContext.SimilarPostSources = []model.PostRecallSource{
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return nil, errors.New("recall store down")
		}},
		fakePostSource{name: "milvus", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(12, 3, "tech", 0.7)}, nil
		}},
	}
	serviceContext.ContentService = &fakeRecommendContentService{unpublished: map[int64]struct{}{}}
	response, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).GetSimilarPosts(similarRequest())
	if err != nil {
		t.Fatalf("GetSimilarPosts() error = %v", err)
	}
	if len(response.Posts) != 1 || response.Posts[0].PostId != 12 {
		t.Fatalf("unexpected posts: %+v", response.Posts)
	}
	version := response.Posts[0].ModelVersion
	for _, mark := range []string{"recall-degraded", "feature-degraded"} {
		if !strings.Contains(version, mark) {
			t.Fatalf("model version %q missing %q", version, mark)
		}
	}
}

func TestGetSimilarPostsLogsInferenceDegradation(t *testing.T) {
	serviceContext := newTestServiceContext(t, time.Unix(1_800_000_000, 0))
	serviceContext.Config.OnlineInfer.Enabled = true
	serviceContext.Config.OnlineInfer.ModelVersion = "rank-v1"
	// 推理返回分数缺失：按 infer-invalid 降级为规则分，不失败整个请求。
	serviceContext.InferenceRanker = fakeInferenceRanker{
		rank: func(context.Context, string, string, []model.PostCandidate) (model.InferenceResult, error) {
			return model.InferenceResult{ModelVersion: "rank-v1", Scores: map[int64]float64{}}, nil
		},
	}
	serviceContext.SimilarPostSources = []model.PostRecallSource{
		fakePostSource{name: "itemcf", recall: func(context.Context, model.RecallRequest) ([]model.PostCandidate, error) {
			return []model.PostCandidate{knownPost(11, 2, "tech", 0.8)}, nil
		}},
	}
	response, err := NewGetSimilarPostsLogic(context.Background(), serviceContext).GetSimilarPosts(similarRequest())
	if err != nil {
		t.Fatalf("GetSimilarPosts() error = %v", err)
	}
	if len(response.Posts) != 1 {
		t.Fatalf("unexpected posts: %+v", response.Posts)
	}
	if !strings.Contains(response.Posts[0].ModelVersion, "infer-invalid") {
		t.Fatalf("expected infer-invalid degradation, got %q", response.Posts[0].ModelVersion)
	}
}
