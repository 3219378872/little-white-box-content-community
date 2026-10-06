package logic

import (
	"context"
	"fmt"
	"strings"

	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/internal/svc"
	pb "esx/kitex_gen/recommend"
	"esx/pkg/errx"

	"esx/pkg/logging"
)

// GetSimilarPostsLogic 承载 GetSimilarPosts 接口的业务逻辑；每个请求新建一个实例。
type GetSimilarPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetSimilarPostsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetSimilarPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSimilarPostsLogic {
	return &GetSimilarPostsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logging.WithContext(ctx)}
}

// GetSimilarPosts 以指定帖子为种子召回相似帖子，排除种子本身，只返回已发布的帖子。
func (l *GetSimilarPostsLogic) GetSimilarPosts(in *pb.GetSimilarPostsReq) (*pb.GetSimilarPostsResp, error) {
	if in == nil || l.svcCtx == nil || in.GetPostId() <= 0 ||
		!validRequestMetadata(in.GetRequestId(), in.GetScene(), "", in.GetExperimentId()) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	limit, err := configuredPageSize(in.GetLimit(), l.svcCtx.Config)
	if err != nil {
		return nil, err
	}
	requestID := strings.TrimSpace(in.GetRequestId())
	recallReq := model.RecallRequest{
		Scene:        normalizedScene(in.GetScene()),
		RequestID:    requestID,
		ExperimentID: strings.TrimSpace(in.GetExperimentId()),
		SeedPostID:   in.GetPostId(),
		Limit:        candidateLimit(limit, l.svcCtx.Config),
	}
	batches, recallDegraded, err := recallPosts(l.ctx, l.svcCtx.SimilarPostSources, recallReq)
	if err != nil {
		recommendPipelineTotal.Inc("similar_posts", "recall", "unavailable")
		l.Errorw("similar post recall unavailable", logging.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("similar_posts", "recall", recallDegraded)
	candidates := mergePostCandidates(batches, recallReq.Limit)
	recommendRecallCandidates.Observe(int64(len(candidates)), "similar_posts")
	if len(candidates) == 0 && recallDegraded {
		recordRecommendationResult("similar_posts", 0)
		return nil, recommendationRPCError(fmt.Errorf("no similar post candidates remain after partial recall failure"))
	}
	candidates, featureDegraded, err := enrichAndFilterPosts(
		l.ctx, l.svcCtx.FeatureRepository, "", candidates, in.GetPostId(), false,
	)
	if err != nil {
		recommendPipelineTotal.Inc("similar_posts", "features", "unavailable")
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("similar_posts", "features", featureDegraded)
	candidates, inferenceDegradation, err := applyInference(
		l.ctx, l.svcCtx.Config, l.svcCtx.InferenceRanker, "similar_posts", requestID, candidates,
	)
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	candidates = rerankPosts(candidates, l.svcCtx.Config.ExploreRatio, l.svcCtx.Config.MaxPerAuthor, requestID)
	candidates, err = filterPublishedPostCandidates(l.ctx, l.svcCtx.ContentService, candidates)
	if err != nil {
		recommendPipelineTotal.Inc("similar_posts", "visibility", "unavailable")
		l.Errorw("similar post visibility check unavailable", logging.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	if recallDegraded {
		markPostDegradation(candidates, "recall-degraded")
	}
	if featureDegraded {
		markPostDegradation(candidates, "feature-degraded")
	}
	if inferenceDegradation != "" {
		l.Errorw("similar post inference degraded", logging.Field("mode", inferenceDegradation))
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	posts := make([]*pb.RecommendPost, 0, len(candidates))
	for index, candidate := range candidates {
		posts = append(posts, &pb.RecommendPost{
			PostId: candidate.PostID, Score: candidate.FinalScore, Reason: candidate.Reason,
			RecallSource: candidate.RecallSource, ModelVersion: candidate.ModelVersion,
			ExperimentId: recallReq.ExperimentID, Position: int32(index + 1),
		})
	}
	recordRecommendationResult("similar_posts", len(posts))
	return &pb.GetSimilarPostsResp{Posts: posts, RequestId: requestID}, nil
}
