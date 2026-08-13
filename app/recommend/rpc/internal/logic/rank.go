package logic

import (
	"context"
	"errors"
	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/model"
	"math"
	"sort"
	"strings"
	"time"
)

func coarseRankPosts(candidates []model.PostCandidate, modelVersion string) []model.PostCandidate {
	if modelVersion == "" {
		modelVersion = "rules-v2"
	}
	maxRecall := 0.0
	for _, candidate := range candidates {
		maxRecall = math.Max(maxRecall, candidate.RecallScore)
	}
	for index := range candidates {
		recall := 0.0
		if maxRecall > 0 {
			recall = candidates[index].RecallScore / maxRecall
		}
		features := candidates[index].Features
		score := recall*0.50 + boundedFeature(features.Quality)*0.18 +
			boundedFeature(features.CTR)*0.12 + boundedFeature(features.Freshness)*0.10 +
			boundedFeature(features.Popularity)*0.10
		candidates[index].CoarseScore = score
		candidates[index].FinalScore = score
		candidates[index].ModelVersion = modelVersion
	}
	sortPostCandidates(candidates)
	return candidates
}

func applyInference(
	ctx context.Context,
	c config.Config,
	ranker model.InferenceRanker,
	operation string,
	requestID string,
	candidates []model.PostCandidate,
) ([]model.PostCandidate, string, error) {
	candidates = coarseRankPosts(candidates, c.RuleModelVersion)
	if len(candidates) == 0 {
		recommendInferenceTotal.Inc(operation, "empty_candidates")
		return candidates, "", nil
	}
	if !c.OnlineInfer.Enabled {
		recommendInferenceTotal.Inc(operation, "disabled")
		return candidates, "", nil
	}
	if ranker == nil {
		recommendInferenceTotal.Inc(operation, "unavailable")
		setPostModelVersion(candidates, appendDegradation(ruleModelVersion(c), "infer-unavailable"))
		return candidates, "infer-unavailable", nil
	}
	timeout := time.Duration(c.OnlineInfer.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultInferenceTimeout
	}
	rankCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := ranker.Rank(rankCtx, requestID, c.OnlineInfer.ModelVersion, candidates)
	if err != nil {
		if ctx.Err() != nil {
			recommendInferenceTotal.Inc(operation, "canceled")
			return nil, "", ctx.Err()
		}
		degradation := "infer-unavailable"
		if errors.Is(rankCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			degradation = "infer-timeout"
		}
		recommendInferenceTotal.Inc(operation, strings.TrimPrefix(degradation, "infer-"))
		setPostModelVersion(candidates, appendDegradation(ruleModelVersion(c), degradation))
		return candidates, degradation, nil
	}
	if len(result.Scores) != len(candidates) || result.ModelVersion == "" {
		recommendInferenceTotal.Inc(operation, "invalid")
		setPostModelVersion(candidates, appendDegradation(ruleModelVersion(c), "infer-invalid"))
		return candidates, "infer-invalid", nil
	}
	for index := range candidates {
		score, ok := result.Scores[candidates[index].PostID]
		if !ok || math.IsNaN(score) || math.IsInf(score, 0) {
			recommendInferenceTotal.Inc(operation, "invalid")
			setPostModelVersion(candidates, appendDegradation(ruleModelVersion(c), "infer-invalid"))
			return candidates, "infer-invalid", nil
		}
		candidates[index].FinalScore = score
		candidates[index].ModelVersion = result.ModelVersion
	}
	sortPostCandidates(candidates)
	recommendInferenceTotal.Inc(operation, "success")
	return candidates, "", nil
}

func rankUsers(candidates []model.UserCandidate, modelVersion string) []model.UserCandidate {
	if modelVersion == "" {
		modelVersion = "rules-v2"
	}
	maxRecall := 0.0
	for _, candidate := range candidates {
		maxRecall = math.Max(maxRecall, candidate.RecallScore)
	}
	for index := range candidates {
		recall := 0.0
		if maxRecall > 0 {
			recall = candidates[index].RecallScore / maxRecall
		}
		features := candidates[index].Features
		candidates[index].FinalScore = recall*0.55 + boundedFeature(features.Quality)*0.20 +
			boundedFeature(features.MutualCount)*0.15 + boundedFeature(features.InterestAffinity)*0.10
		candidates[index].ModelVersion = modelVersion
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FinalScore == candidates[j].FinalScore {
			return candidates[i].UserID < candidates[j].UserID
		}
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
	return candidates
}

func rerankPosts(candidates []model.PostCandidate, ratio float64, maxPerAuthor int, seed string) []model.PostCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 0.5 {
		ratio = 0.5
	}
	if maxPerAuthor <= 0 {
		maxPerAuthor = 2
	}
	remaining := append([]model.PostCandidate(nil), candidates...)
	result := make([]model.PostCandidate, 0, len(candidates))
	authorCounts := make(map[int64]int)
	exploreTarget := int(math.Ceil(float64(len(candidates)) * ratio))
	explorePicked := 0
	lastCategory := ""
	for len(remaining) > 0 {
		bestIndex := 0
		bestScore := math.Inf(-1)
		for index, candidate := range remaining {
			score := candidate.FinalScore + deterministicJitter(seed, candidate.PostID)
			if candidate.AuthorID > 0 && authorCounts[candidate.AuthorID] >= maxPerAuthor {
				score -= 0.35
			}
			if candidate.Category != "" && candidate.Category == lastCategory {
				score -= 0.08
			}
			if explorePicked < exploreTarget && hasSource(candidate.RecallSource, "explore") {
				score += 0.12
			}
			if score > bestScore {
				bestScore = score
				bestIndex = index
			}
		}
		chosen := remaining[bestIndex]
		result = append(result, chosen)
		if chosen.AuthorID > 0 {
			authorCounts[chosen.AuthorID]++
		}
		if hasSource(chosen.RecallSource, "explore") {
			explorePicked++
		}
		lastCategory = chosen.Category
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	return result
}

// enforceAuthorQuota 在候选池至少包含 10 个不同作者时，保证任意连续 20 条结果中
// 同一作者最多出现 maxPerAuthor 条（DISC-034）。通过滑窗跳过超限候选实现，保持排序稳定。
func enforceAuthorQuota(posts []model.PostCandidate, maxPerAuthor int) []model.PostCandidate {
	distinctAuthors := make(map[int64]struct{}, len(posts))
	for _, post := range posts {
		if post.AuthorID > 0 {
			distinctAuthors[post.AuthorID] = struct{}{}
		}
	}
	if len(distinctAuthors) < 10 || maxPerAuthor <= 0 {
		return posts
	}
	result := make([]model.PostCandidate, 0, len(posts))
	window := make([]int64, 0, 20)
	for _, post := range posts {
		if post.AuthorID <= 0 {
			result = append(result, post)
			window = append(window, 0)
			if len(window) > 20 {
				window = window[len(window)-20:]
			}
			continue
		}
		authorCount := 0
		for _, authorID := range window {
			if authorID == post.AuthorID {
				authorCount++
			}
		}
		if authorCount >= maxPerAuthor {
			continue
		}
		result = append(result, post)
		window = append(window, post.AuthorID)
		if len(window) > 20 {
			window = window[len(window)-20:]
		}
	}
	return result
}

func rerankUsers(candidates []model.UserCandidate, ratio float64, seed string) []model.UserCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	remaining := append([]model.UserCandidate(nil), candidates...)
	result := make([]model.UserCandidate, 0, len(candidates))
	exploreTarget := int(math.Ceil(float64(len(candidates)) * math.Max(0, math.Min(ratio, 0.5))))
	explorePicked := 0
	lastCategory := ""
	for len(remaining) > 0 {
		bestIndex := 0
		bestScore := math.Inf(-1)
		for index, candidate := range remaining {
			score := candidate.FinalScore + deterministicJitter(seed, candidate.UserID)
			if candidate.Category != "" && candidate.Category == lastCategory {
				score -= 0.08
			}
			if explorePicked < exploreTarget && hasSource(candidate.RecallSource, "explore") {
				score += 0.12
			}
			if score > bestScore {
				bestScore = score
				bestIndex = index
			}
		}
		chosen := remaining[bestIndex]
		result = append(result, chosen)
		if hasSource(chosen.RecallSource, "explore") {
			explorePicked++
		}
		lastCategory = chosen.Category
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	return result
}
