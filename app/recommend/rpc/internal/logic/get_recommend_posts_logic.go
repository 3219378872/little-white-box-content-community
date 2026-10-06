package logic

import (
	"context"
	"errors"
	"esx/app/recommend/featurekey"
	"esx/app/recommend/rpc/internal/cursor"
	"esx/app/recommend/rpc/internal/model"
	"esx/app/recommend/rpc/internal/svc"
	pb "esx/kitex_gen/recommend"
	"esx/pkg/errx"
	"fmt"
	"strconv"
	"strings"

	"esx/pkg/logging"
)

// GetRecommendPostsLogic 承载 GetRecommendPosts 接口的业务逻辑；每个请求新建一个实例。
type GetRecommendPostsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logging.Logger
}

// NewGetRecommendPostsLogic 绑定请求上下文与服务依赖，日志自动携带请求追踪信息。
func NewGetRecommendPostsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendPostsLogic {
	return &GetRecommendPostsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logging.WithContext(ctx),
	}
}

// GetRecommendPosts 返回推荐帖子一页。首屏走完整管线：召回 → 特征补全与过滤 → 推理排序与重排 →
// 可见性校验 → 冻结为快照；翻页只从快照读取，保证同一次推荐内顺序稳定、不重复。
func (l *GetRecommendPostsLogic) GetRecommendPosts(in *pb.GetRecommendPostsReq) (*pb.GetRecommendPostsResp, error) {
	if in == nil || l.svcCtx == nil || !validIdentity(in.GetUserId(), in.GetAnonymousId()) ||
		!validRequestMetadata(in.GetRequestId(), in.GetScene(), in.GetSessionId(), in.GetExperimentId()) {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	pageSize, err := configuredPageSize(in.GetPageSize(), l.svcCtx.Config)
	if err != nil {
		return nil, err
	}
	identity := featurekey.Identity(in.GetUserId(), strings.TrimSpace(in.GetAnonymousId()))
	binding := recommendationBinding(in, identity, pageSize)
	if in.GetCursor() != "" {
		return l.pageFromCursor(in.GetCursor(), pageSize, binding, identity)
	}

	// 关闭个性化的用户与匿名用户只能使用规则来源，快照也据此标记为 rule-only。
	privacyOptOut := l.personalizationOptedOut(in.GetUserId())
	ruleOnly := privacyOptOut || in.GetUserId() <= 0
	recallReq := model.RecallRequest{
		UserID:       in.GetUserId(),
		AnonymousID:  strings.TrimSpace(in.GetAnonymousId()),
		Identity:     identity,
		Scene:        binding.Scene,
		RequestID:    binding.RequestID,
		SessionID:    binding.SessionID,
		ExperimentID: binding.ExperimentID,
		Limit:        candidateLimit(pageSize, l.svcCtx.Config),
	}
	candidates, recallDegraded, err := l.recallCandidates(recallReq, ruleOnly)
	if err != nil {
		return nil, err
	}

	// 补全观看者与帖子特征，并过滤负反馈、已曝光与屏蔽作者的内容。
	candidates, featureDegraded, err := enrichAndFilterPosts(
		l.ctx, l.svcCtx.FeatureRepository, identity, candidates, 0, privacyOptOut,
	)
	if err != nil {
		recommendPipelineTotal.Inc("posts", "features", "unavailable")
		l.Errorw("post feature enrichment unavailable", logging.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	recordPipelineStage("posts", "features", featureDegraded)
	if len(candidates) == 0 {
		recordRecommendationResult("posts", 0)
		return &pb.GetRecommendPostsResp{Posts: []*pb.RecommendPost{}, RequestId: binding.RequestID}, nil
	}
	candidates, err = l.rankVisiblePosts(candidates, binding, identity)
	if err != nil {
		return nil, err
	}

	// 降级原因写进推荐理由，便于客户端与离线分析识别非完整推荐。
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
	response, err := l.firstPage(rankedPosts(candidates, binding.ExperimentID), pageSize, binding, ruleOnly)
	if err != nil {
		return nil, err
	}
	recordRecommendationResult("posts", len(response.Posts))
	return response, nil
}

// recommendationBinding 汇总游标必须绑定的请求属性；翻页时任一属性变化都会让游标失效。
func recommendationBinding(in *pb.GetRecommendPostsReq, identity string, pageSize int) cursor.Binding {
	return cursor.Binding{
		IdentityHash: cursor.IdentityHash(identity),
		RequestID:    strings.TrimSpace(in.GetRequestId()),
		Scene:        normalizedScene(in.GetScene()),
		SessionID:    strings.TrimSpace(in.GetSessionId()),
		ExperimentID: strings.TrimSpace(in.GetExperimentId()),
		PageSize:     pageSize,
	}
}

// recallCandidates 并行调用召回来源并合并去重。部分来源失败时继续用其余来源（标记降级），
// 但若失败后一个候选都没有，则返回错误而不是空推荐。
func (l *GetRecommendPostsLogic) recallCandidates(req model.RecallRequest, ruleOnly bool) ([]model.PostCandidate, bool, error) {
	sources := l.svcCtx.PostRecallSources
	// DISC-031：匿名用户只使用热门/最新/标签等非持久化冷启动来源。
	if ruleOnly {
		sources = ruleOnlyPostSources(sources)
	}
	batches, recallDegraded, err := recallPosts(l.ctx, sources, req)
	if err != nil {
		recommendPipelineTotal.Inc("posts", "recall", "unavailable")
		l.Errorw("post recall unavailable", logging.Field("err", err.Error()))
		return nil, false, recommendationRPCError(err)
	}
	recordPipelineStage("posts", "recall", recallDegraded)
	candidates := mergePostCandidates(batches, req.Limit)
	recommendRecallCandidates.Observe(int64(len(candidates)), "posts")
	if len(candidates) == 0 && recallDegraded {
		recordRecommendationResult("posts", 0)
		return nil, false, recommendationRPCError(fmt.Errorf("no post candidates remain after partial recall failure"))
	}
	return candidates, recallDegraded, nil
}

// rankedPosts 把最终候选转换为带位置的结果，位置从 1 开始，供曝光与点击归因。
func rankedPosts(candidates []model.PostCandidate, experimentID string) []model.RankedPost {
	ranked := make([]model.RankedPost, 0, len(candidates))
	for index, candidate := range candidates {
		ranked = append(ranked, model.RankedPost{
			PostID:       candidate.PostID,
			Score:        candidate.FinalScore,
			Reason:       candidate.Reason,
			RecallSource: candidate.RecallSource,
			ModelVersion: candidate.ModelVersion,
			ExperimentID: experimentID,
			Position:     int32(index + 1),
		})
	}
	return ranked
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
		l.Errorw("check personalization opt-out failed", logging.Field("user_id", userID), logging.Field("err", err.Error()))
		return true
	}
	return optedOut
}

// firstPage 返回首屏，并在还有更多结果时把完整排序结果存为快照、生成指向快照的游标。
func (l *GetRecommendPostsLogic) firstPage(posts []model.RankedPost, pageSize int, binding cursor.Binding, ruleOnly bool) (*pb.GetRecommendPostsResp, error) {
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
	response.NextCursor, err = l.saveSnapshotCursor(posts, end, binding, ruleOnly)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// saveSnapshotCursor 保存排序快照并编码从 offset 开始的游标；快照与游标使用同一过期时间。
func (l *GetRecommendPostsLogic) saveSnapshotCursor(posts []model.RankedPost, offset int, binding cursor.Binding, ruleOnly bool) (string, error) {
	if l.svcCtx.SnapshotStore == nil || l.svcCtx.CursorCodec == nil || l.svcCtx.NewSnapshotID == nil {
		return "", errx.NewWithCode(errx.ServiceUnavailable)
	}
	snapshotID, err := l.svcCtx.NewSnapshotID()
	if err != nil {
		return "", recommendationRPCError(err)
	}
	now := l.svcCtx.Now
	if now == nil {
		return "", errx.NewWithCode(errx.ServiceUnavailable)
	}
	expiresAt := now().Unix() + int64(cursorTTL(l.svcCtx.Config))
	snapshot := model.PostSnapshot{
		RuleOnly:     ruleOnly,
		RequestID:    binding.RequestID,
		IdentityHash: binding.IdentityHash,
		Scene:        binding.Scene,
		SessionID:    binding.SessionID,
		ExperimentID: binding.ExperimentID,
		ExpiresAt:    expiresAt,
		Posts:        posts,
	}
	if err := l.svcCtx.SnapshotStore.Save(l.ctx, snapshotID, snapshot, cursorTTL(l.svcCtx.Config)); err != nil {
		l.Errorw("save recommendation snapshot failed", logging.Field("err", err.Error()))
		return "", recommendationRPCError(err)
	}
	token, err := l.svcCtx.CursorCodec.Encode(snapshotID, offset, expiresAt, binding)
	if err != nil {
		return "", errx.Wrap(err, errx.SystemError)
	}
	return token, nil
}

// pageFromCursor 从快照读取后续页：快照冻结排序，但每页仍重新校验可见性与最新的负反馈。
func (l *GetRecommendPostsLogic) pageFromCursor(token string, pageSize int, binding cursor.Binding, identity string) (*pb.GetRecommendPostsResp, error) {
	payload, snapshot, err := l.loadBoundSnapshot(token, binding)
	if err != nil {
		return nil, err
	}
	// Personalization may have been disabled since the snapshot was created.
	// Old serialized snapshots lack RuleOnly and are conservatively invalidated.
	if strings.HasPrefix(identity, featurekey.UserIdentityPrefix) && !snapshot.RuleOnly {
		userID, err := strconv.ParseInt(strings.TrimPrefix(identity, featurekey.UserIdentityPrefix), 10, 64)
		if err != nil || userID <= 0 || l.personalizationOptedOut(userID) {
			return nil, errx.New(errx.ParamError, "recommendation cursor expired after personalization changed")
		}
	}
	if payload.Offset >= len(snapshot.Posts) {
		return nil, errx.New(errx.ParamError, "invalid or expired recommendation cursor")
	}
	visible, err := filterPublishedRankedPosts(l.ctx, l.svcCtx.ContentService, snapshot.Posts[payload.Offset:])
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	visible, err = l.dropHiddenPosts(identity, visible)
	if err != nil {
		return nil, err
	}
	end := min(pageSize, len(visible))
	response := &pb.GetRecommendPostsResp{
		Posts:     recommendPostsToPB(visible[:end]),
		HasMore:   end < len(visible),
		RequestId: binding.RequestID,
	}
	if response.HasMore {
		// 下一页从快照中本页最后一个返回项之后开始，被过滤的项不会重复出现。
		nextOffset := snapshotOffsetAfter(snapshot.Posts, payload.Offset, visible[end-1].PostID)
		response.NextCursor, err = l.svcCtx.CursorCodec.Encode(payload.SnapshotID, nextOffset, payload.ExpiresAt, binding)
		if err != nil {
			return nil, errx.Wrap(err, errx.SystemError)
		}
	}
	return response, nil
}

// loadBoundSnapshot 解码游标并加载快照，要求快照仍与本次请求的身份、场景、会话、实验与过期时间一致。
func (l *GetRecommendPostsLogic) loadBoundSnapshot(token string, binding cursor.Binding) (cursor.Payload, model.PostSnapshot, error) {
	if l.svcCtx.CursorCodec == nil || l.svcCtx.SnapshotStore == nil {
		return cursor.Payload{}, model.PostSnapshot{}, errx.NewWithCode(errx.ServiceUnavailable)
	}
	payload, err := l.svcCtx.CursorCodec.Decode(token, binding)
	if err != nil {
		return cursor.Payload{}, model.PostSnapshot{}, errx.New(errx.ParamError, "invalid or expired recommendation cursor")
	}
	snapshot, err := l.svcCtx.SnapshotStore.Load(l.ctx, payload.SnapshotID)
	if err != nil {
		if errors.Is(err, model.ErrSnapshotMissing) {
			return cursor.Payload{}, model.PostSnapshot{}, errx.New(errx.ParamError, "invalid or expired recommendation cursor")
		}
		return cursor.Payload{}, model.PostSnapshot{}, recommendationRPCError(err)
	}
	if snapshot.RequestID != binding.RequestID || snapshot.IdentityHash != binding.IdentityHash ||
		snapshot.Scene != binding.Scene || snapshot.SessionID != binding.SessionID ||
		snapshot.ExperimentID != binding.ExperimentID || snapshot.ExpiresAt != payload.ExpiresAt {
		return cursor.Payload{}, model.PostSnapshot{}, recommendationRPCError(fmt.Errorf("recommendation snapshot binding is invalid"))
	}
	return payload, snapshot, nil
}

// dropHiddenPosts 去掉登录用户在首屏之后才负反馈的帖子。
// A snapshot freezes ranking, not explicit feedback. Another tab can hide
// a remaining candidate after the first page was returned (DISC-035).
func (l *GetRecommendPostsLogic) dropHiddenPosts(identity string, visible []model.RankedPost) ([]model.RankedPost, error) {
	if !strings.HasPrefix(identity, featurekey.UserIdentityPrefix) {
		return visible, nil
	}
	if l.svcCtx.FeatureRepository == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	viewer, err := l.svcCtx.FeatureRepository.LoadViewerFeatures(l.ctx, identity)
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	filtered := make([]model.RankedPost, 0, len(visible))
	for _, post := range visible {
		if !containsID(viewer.NegativePostIDs, post.PostID) {
			filtered = append(filtered, post)
		}
	}
	return filtered, nil
}

// snapshotOffsetAfter 从 from 开始在快照中找到 lastID，返回其后一位的偏移。
func snapshotOffsetAfter(posts []model.RankedPost, from int, lastID int64) int {
	next := from
	for next < len(posts) {
		post := posts[next]
		next++
		if post.PostID == lastID {
			break
		}
	}
	return next
}

// recommendPostsToPB 把排序结果转换为 RPC 响应项。
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

// rankVisiblePosts 依次执行在线推理打分、探索重排、作者配额与可见性过滤；
// 推理降级时沿用规则分，不中断推荐。
func (l *GetRecommendPostsLogic) rankVisiblePosts(candidates []model.PostCandidate, binding cursor.Binding, identity string) ([]model.PostCandidate, error) {
	candidates, inferenceDegradation, err := applyInference(
		l.ctx, l.svcCtx.Config, l.svcCtx.InferenceRanker, "posts", binding.RequestID, candidates,
	)
	if err != nil {
		return nil, recommendationRPCError(err)
	}
	if inferenceDegradation != "" {
		l.Errorw("online inference degraded", logging.Field("mode", inferenceDegradation))
	}
	candidates = rerankPosts(
		candidates, l.svcCtx.Config.ExploreRatio, l.svcCtx.Config.MaxPerAuthor,
		binding.RequestID+":"+identity,
	)
	candidates = enforceAuthorQuota(candidates, l.svcCtx.Config.MaxPerAuthor)
	candidates, err = filterPublishedPostCandidates(l.ctx, l.svcCtx.ContentService, candidates)
	if err != nil {
		recommendPipelineTotal.Inc("posts", "visibility", "unavailable")
		l.Errorw("post visibility check unavailable", logging.Field("err", err.Error()))
		return nil, recommendationRPCError(err)
	}
	return candidates, nil
}
