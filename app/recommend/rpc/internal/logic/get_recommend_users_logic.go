package logic

import (
	"context"
	"fmt"
	"strings"

	"errx"
	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/internal/svc"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRecommendUsersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetRecommendUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendUsersLogic {
	return &GetRecommendUsersLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetRecommendUsersLogic) GetRecommendUsers(in *pb.GetRecommendUsersReq) (*pb.GetRecommendUsersResp, error) {
	if in == nil || l.svcCtx == nil || !validIdentity(in.GetUserId(), in.GetAnonymousId()) ||
		!validRequestMetadata(in.GetRequestId(), in.GetScene(), "", in.GetExperimentId()) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	limit, err := configuredPageSize(in.GetLimit(), l.svcCtx.Config)
	if err != nil {
		return nil, err
	}
	identity := model.IdentityKey(in.GetUserId(), strings.TrimSpace(in.GetAnonymousId()))
	requestID := strings.TrimSpace(in.GetRequestId())
	recallReq := model.RecallRequest{
		UserID:       in.GetUserId(),
		AnonymousID:  strings.TrimSpace(in.GetAnonymousId()),
		Identity:     identity,
		Scene:        normalizedScene(in.GetScene()),
		RequestID:    requestID,
		ExperimentID: strings.TrimSpace(in.GetExperimentId()),
		Limit:        candidateLimit(limit, l.svcCtx.Config),
	}
	batches, recallDegraded, err := recallUsers(l.ctx, l.svcCtx.UserRecallSources, recallReq)
	if err != nil {
		recommendPipelineTotal.Inc("users", "recall", "unavailable")
		l.Errorw("user recall unavailable", logx.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("users", "recall", recallDegraded)
	candidates := mergeUserCandidates(batches, recallReq.Limit)
	recommendRecallCandidates.Observe(int64(len(candidates)), "users")
	if len(candidates) == 0 && recallDegraded {
		recordRecommendationResult("users", 0)
		return nil, recommendationRPCError(fmt.Errorf("no user candidates remain after partial recall failure"))
	}
	candidates, featureDegraded, err := enrichAndFilterUsers(
		l.ctx, l.svcCtx.FeatureRepository, in.GetUserId(), candidates,
	)
	if err != nil {
		recommendPipelineTotal.Inc("users", "features", "unavailable")
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("users", "features", featureDegraded)
	candidates = rankUsers(candidates, ruleModelVersion(l.svcCtx.Config))
	candidates = rerankUsers(candidates, l.svcCtx.Config.ExploreRatio, requestID+":"+identity)
	if recallDegraded {
		markUserDegradation(candidates, "recall-degraded")
	}
	if featureDegraded {
		markUserDegradation(candidates, "feature-degraded")
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	users := make([]*pb.RecommendUser, 0, len(candidates))
	for index, candidate := range candidates {
		users = append(users, &pb.RecommendUser{
			UserId: candidate.UserID, Score: candidate.FinalScore, Reason: candidate.Reason,
			RecallSource: candidate.RecallSource, ModelVersion: candidate.ModelVersion,
			ExperimentId: recallReq.ExperimentID, Position: int32(index + 1),
		})
	}
	recordRecommendationResult("users", len(users))
	return &pb.GetRecommendUsersResp{Users: users, RequestId: requestID}, nil
}
