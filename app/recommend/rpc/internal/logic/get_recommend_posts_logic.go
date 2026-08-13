package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"errx"
	"esx/app/recommend/rpc/internal/cursor"
	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/internal/svc"
	"esx/app/recommend/rpc/xiaobaihe/recommend/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRecommendPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetRecommendPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendPostsLogic {
	return &GetRecommendPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetRecommendPostsLogic) GetRecommendPosts(in *pb.GetRecommendPostsReq) (*pb.GetRecommendPostsResp, error) {
	if in == nil || l.svcCtx == nil || !validIdentity(in.GetUserId(), in.GetAnonymousId()) ||
		!validRequestMetadata(in.GetRequestId(), in.GetScene(), in.GetSessionId(), in.GetExperimentId()) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	pageSize, err := configuredPageSize(in.GetPageSize(), l.svcCtx.Config)
	if err != nil {
		return nil, err
	}
	identity := model.IdentityKey(in.GetUserId(), strings.TrimSpace(in.GetAnonymousId()))
	scene := normalizedScene(in.GetScene())
	binding := cursor.Binding{
		IdentityHash: cursor.IdentityHash(identity),
		RequestID:    strings.TrimSpace(in.GetRequestId()),
		Scene:        scene,
		SessionID:    strings.TrimSpace(in.GetSessionId()),
		ExperimentID: strings.TrimSpace(in.GetExperimentId()),
		PageSize:     pageSize,
	}
	if in.GetCursor() != "" {
		return l.pageFromCursor(in.GetCursor(), pageSize, binding)
	}

	privacyOptOut := l.personalizationOptedOut(in.GetUserId())
	recallReq := model.RecallRequest{
		UserID:       in.GetUserId(),
		AnonymousID:  strings.TrimSpace(in.GetAnonymousId()),
		Identity:     identity,
		Scene:        scene,
		RequestID:    binding.RequestID,
		SessionID:    binding.SessionID,
		ExperimentID: binding.ExperimentID,
		Limit:        candidateLimit(pageSize, l.svcCtx.Config),
	}
	sources := l.svcCtx.PostRecallSources
	// DISC-031：匿名用户只使用热门/最新/标签等非持久化冷启动来源。
	if privacyOptOut || in.GetUserId() <= 0 {
		sources = ruleOnlyPostSources(sources)
	}
	batches, recallDegraded, err := recallPosts(l.ctx, sources, recallReq)
	if err != nil {
		recommendPipelineTotal.Inc("posts", "recall", "unavailable")
		l.Errorw("post recall unavailable", logx.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("posts", "recall", recallDegraded)
	candidates := mergePostCandidates(batches, recallReq.Limit)
	recommendRecallCandidates.Observe(int64(len(candidates)), "posts")
	if len(candidates) == 0 && recallDegraded {
		recordRecommendationResult("posts", 0)
		return nil, recommendationRPCError(fmt.Errorf("no post candidates remain after partial recall failure"))
	}
	candidates, featureDegraded, err := enrichAndFilterPosts(
		l.ctx, l.svcCtx.FeatureRepository, identity, candidates, 0, privacyOptOut,
	)
	if err != nil {
		recommendPipelineTotal.Inc("posts", "features", "unavailable")
		l.Errorw("post feature enrichment unavailable", logx.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("posts", "features", featureDegraded)
	if len(candidates) == 0 {
		recordRecommendationResult("posts", 0)
		return &pb.GetRecommendPostsResp{Posts: []*pb.RecommendPost{}, RequestId: binding.RequestID}, nil
	}
	candidates, inferenceDegradation, err := applyInference(
		l.ctx, l.svcCtx.Config, l.svcCtx.InferenceRanker, "posts", binding.RequestID, candidates,
	)
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	if inferenceDegradation != "" {
		l.Errorw("online inference degraded", logx.Field("mode", inferenceDegradation))
	}
	candidates = rerankPosts(
		candidates, l.svcCtx.Config.ExploreRatio, l.svcCtx.Config.MaxPerAuthor,
		binding.RequestID+":"+identity,
	)
	candidates = enforceAuthorQuota(candidates, l.svcCtx.Config.MaxPerAuthor)
	candidates, err = filterPublishedPostCandidates(l.ctx, l.svcCtx.ContentService, candidates)
	if err != nil {
		recommendPipelineTotal.Inc("posts", "visibility", "unavailable")
		l.Errorw("post visibility check unavailable", logx.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	if recallDegraded {
		markPostDegradation(candidates, "recall-degraded")
		l.Error("one or more post recall sources failed; serving remaining sources")
	}
	if featureDegraded {
		markPostDegradation(candidates, "feature-degraded")
		l.Error("recommendation features degraded; serving verified candidates only")
	}
	if privacyOptOut {
		markPostDegradation(candidates, "personalization-disabled")
	}
	ranked := make([]model.RankedPost, 0, len(candidates))
	for index, candidate := range candidates {
		ranked = append(ranked, model.RankedPost{
			PostID:       candidate.PostID,
			Score:        candidate.FinalScore,
			Reason:       candidate.Reason,
			RecallSource: candidate.RecallSource,
			ModelVersion: candidate.ModelVersion,
			ExperimentID: binding.ExperimentID,
			Position:     int32(index + 1),
		})
	}
	response, err := l.firstPage(ranked, pageSize, binding)
	if err != nil {
		return nil, err
	}
	recordRecommendationResult("posts", len(response.Posts))
	return response, nil
}

// personalizationOptedOut 返回认证用户是否关闭了个性化（REL-023）。
// 偏好无法读取时 fail-closed，只走规则冷启动，避免继续个性化。
func (l *GetRecommendPostsLogic) personalizationOptedOut(userID int64) bool {
	if userID <= 0 {
		return false
	}
	if l.svcCtx == nil || l.svcCtx.FeatureRepository == nil {
		return true
	}
	optedOut, err := l.svcCtx.FeatureRepository.IsPersonalizationOptedOut(l.ctx, userID)
	if err != nil {
		l.Errorw("check personalization opt-out failed", logx.Field("user_id", userID), logx.Field("err", err.Error()))
		return true
	}
	return optedOut
}

func (l *GetRecommendPostsLogic) firstPage(posts []model.RankedPost, pageSize int, binding cursor.Binding) (*pb.GetRecommendPostsResp, error) {
	posts, err := filterPublishedRankedPosts(l.ctx, l.svcCtx.ContentService, posts)
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	end := min(pageSize, len(posts))
	response := &pb.GetRecommendPostsResp{
		Posts:     recommendPostsToPB(posts[:end]),
		HasMore:   end < len(posts),
		RequestId: binding.RequestID,
	}
	if !response.HasMore {
		return response, nil
	}
	if l.svcCtx.SnapshotStore == nil || l.svcCtx.CursorCodec == nil || l.svcCtx.NewSnapshotID == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	snapshotID, err := l.svcCtx.NewSnapshotID()
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	now := l.svcCtx.Now
	if now == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	expiresAt := now().Unix() + int64(cursorTTL(l.svcCtx.Config))
	snapshot := model.PostSnapshot{
		RequestID:    binding.RequestID,
		IdentityHash: binding.IdentityHash,
		Scene:        binding.Scene,
		SessionID:    binding.SessionID,
		ExperimentID: binding.ExperimentID,
		ExpiresAt:    expiresAt,
		Posts:        posts,
	}
	if err := l.svcCtx.SnapshotStore.Save(l.ctx, snapshotID, snapshot, cursorTTL(l.svcCtx.Config)); err != nil {
		l.Errorw("save recommendation snapshot failed", logx.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	response.NextCursor, err = l.svcCtx.CursorCodec.Encode(snapshotID, end, expiresAt, binding)
	if err != nil {
		return nil, errx.Wrap(err, errx.SystemError)
	}
	return response, nil
}

func (l *GetRecommendPostsLogic) pageFromCursor(token string, pageSize int, binding cursor.Binding) (*pb.GetRecommendPostsResp, error) {
	if l.svcCtx.CursorCodec == nil || l.svcCtx.SnapshotStore == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	payload, err := l.svcCtx.CursorCodec.Decode(token, binding)
	if err != nil {
		return nil, errx.New(errx.ParamError, "invalid or expired recommendation cursor")
	}
	snapshot, err := l.svcCtx.SnapshotStore.Load(l.ctx, payload.SnapshotID)
	if err != nil {
		if errors.Is(err, model.ErrSnapshotMissing) {
			return nil, errx.New(errx.ParamError, "invalid or expired recommendation cursor")
		}
		return nil, recommendationRPCError(err)
	}
	if snapshot.RequestID != binding.RequestID || snapshot.IdentityHash != binding.IdentityHash ||
		snapshot.Scene != binding.Scene || snapshot.SessionID != binding.SessionID ||
		snapshot.ExperimentID != binding.ExperimentID || snapshot.ExpiresAt != payload.ExpiresAt {
		return nil, recommendationRPCError(fmt.Errorf("recommendation snapshot binding is invalid"))
	}
	if payload.Offset >= len(snapshot.Posts) {
		return nil, errx.New(errx.ParamError, "invalid or expired recommendation cursor")
	}
	visible, err := filterPublishedRankedPosts(l.ctx, l.svcCtx.ContentService, snapshot.Posts[payload.Offset:])
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	end := min(pageSize, len(visible))
	response := &pb.GetRecommendPostsResp{
		Posts:     recommendPostsToPB(visible[:end]),
		HasMore:   end < len(visible),
		RequestId: binding.RequestID,
	}
	if response.HasMore {
		response.NextCursor, err = l.svcCtx.CursorCodec.Encode(payload.SnapshotID, payload.Offset+end, payload.ExpiresAt, binding)
		if err != nil {
			return nil, errx.Wrap(err, errx.SystemError)
		}
	}
	return response, nil
}

func recommendPostsToPB(posts []model.RankedPost) []*pb.RecommendPost {
	result := make([]*pb.RecommendPost, 0, len(posts))
	for _, post := range posts {
		result = append(result, &pb.RecommendPost{
			PostId:       post.PostID,
			Score:        post.Score,
			Reason:       post.Reason,
			RecallSource: post.RecallSource,
			ModelVersion: post.ModelVersion,
			ExperimentId: post.ExperimentID,
			Position:     post.Position,
		})
	}
	return result
}
