package logic

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"errx"
	"esx/app/recommend/rpc/internal/config"
	"esx/app/recommend/rpc/internal/model"
)

const (
	defaultPageSize          = 20
	defaultCandidateMultiple = 8
	defaultCursorTTLSeconds  = 600
	defaultInferenceTimeout  = 80 * time.Millisecond
	rrfConstant              = 60.0
)

type postRecallResult struct {
	index      int
	candidates []model.PostCandidate
	err        error
}

type userRecallResult struct {
	index      int
	candidates []model.UserCandidate
	err        error
}

func recallPosts(ctx context.Context, sources []model.PostRecallSource, req model.RecallRequest) ([][]model.PostCandidate, bool, error) {
	if len(sources) == 0 {
		return nil, false, fmt.Errorf("no post recall sources configured")
	}
	results := make(chan postRecallResult, len(sources))
	var group sync.WaitGroup
	for index, source := range sources {
		if source == nil {
			continue
		}
		group.Add(1)
		go func(index int, source model.PostRecallSource) {
			defer group.Done()
			candidates, err := source.Recall(ctx, req)
			for candidateIndex := range candidates {
				if candidates[candidateIndex].RecallSource == "" {
					candidates[candidateIndex].RecallSource = source.Name()
				}
			}
			results <- postRecallResult{index: index, candidates: candidates, err: err}
		}(index, source)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	batches := make([][]model.PostCandidate, len(sources))
	var failures []error
	successes := 0
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, model.ErrNotApplicable) {
				failures = append(failures, result.err)
			}
			continue
		}
		successes++
		batches[result.index] = result.candidates
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if successes == 0 {
		if len(failures) == 0 {
			return nil, false, fmt.Errorf("no applicable post recall sources")
		}
		return nil, false, errors.Join(failures...)
	}
	return batches, len(failures) > 0, nil
}

func recallUsers(ctx context.Context, sources []model.UserRecallSource, req model.RecallRequest) ([][]model.UserCandidate, bool, error) {
	if len(sources) == 0 {
		return nil, false, fmt.Errorf("no user recall sources configured")
	}
	results := make(chan userRecallResult, len(sources))
	var group sync.WaitGroup
	for index, source := range sources {
		if source == nil {
			continue
		}
		group.Add(1)
		go func(index int, source model.UserRecallSource) {
			defer group.Done()
			candidates, err := source.Recall(ctx, req)
			for candidateIndex := range candidates {
				if candidates[candidateIndex].RecallSource == "" {
					candidates[candidateIndex].RecallSource = source.Name()
				}
			}
			results <- userRecallResult{index: index, candidates: candidates, err: err}
		}(index, source)
	}
	go func() {
		group.Wait()
		close(results)
	}()

	batches := make([][]model.UserCandidate, len(sources))
	var failures []error
	successes := 0
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, model.ErrNotApplicable) {
				failures = append(failures, result.err)
			}
			continue
		}
		successes++
		batches[result.index] = result.candidates
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if successes == 0 {
		if len(failures) == 0 {
			return nil, false, fmt.Errorf("no applicable user recall sources")
		}
		return nil, false, errors.Join(failures...)
	}
	return batches, len(failures) > 0, nil
}

func mergePostCandidates(batches [][]model.PostCandidate, limit int) []model.PostCandidate {
	merged := make(map[int64]model.PostCandidate)
	for _, batch := range batches {
		for rank, candidate := range batch {
			if candidate.PostID <= 0 {
				continue
			}
			contribution := 1 / (rrfConstant + float64(rank+1))
			current, exists := merged[candidate.PostID]
			if !exists {
				candidate.RecallScore = contribution
				merged[candidate.PostID] = candidate
				continue
			}
			current.RecallScore += contribution
			current.RecallSource = appendSource(current.RecallSource, candidate.RecallSource)
			if !current.Features.Known && candidate.Features.Known {
				current.Features = candidate.Features
				current.AuthorID = candidate.AuthorID
				current.Category = candidate.Category
			}
			merged[candidate.PostID] = current
		}
	}
	result := make([]model.PostCandidate, 0, len(merged))
	for _, candidate := range merged {
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].RecallScore == result[j].RecallScore {
			return result[i].PostID < result[j].PostID
		}
		return result[i].RecallScore > result[j].RecallScore
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func mergeUserCandidates(batches [][]model.UserCandidate, limit int) []model.UserCandidate {
	merged := make(map[int64]model.UserCandidate)
	for _, batch := range batches {
		for rank, candidate := range batch {
			if candidate.UserID <= 0 {
				continue
			}
			contribution := 1 / (rrfConstant + float64(rank+1))
			current, exists := merged[candidate.UserID]
			if !exists {
				candidate.RecallScore = contribution
				merged[candidate.UserID] = candidate
				continue
			}
			current.RecallScore += contribution
			current.RecallSource = appendSource(current.RecallSource, candidate.RecallSource)
			if !current.Features.Known && candidate.Features.Known {
				current.Features = candidate.Features
				current.Category = candidate.Category
			}
			merged[candidate.UserID] = current
		}
	}
	result := make([]model.UserCandidate, 0, len(merged))
	for _, candidate := range merged {
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].RecallScore == result[j].RecallScore {
			return result[i].UserID < result[j].UserID
		}
		return result[i].RecallScore > result[j].RecallScore
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result
}

func enrichAndFilterPosts(
	ctx context.Context,
	repository model.FeatureRepository,
	identity string,
	candidates []model.PostCandidate,
	excludedPostID int64,
) ([]model.PostCandidate, bool, error) {
	viewer := emptyViewerFeatures()
	degraded := false
	if repository == nil {
		degraded = true
	} else {
		loadedViewer, err := repository.LoadViewerFeatures(ctx, identity)
		if err != nil {
			degraded = true
		} else {
			viewer = loadedViewer
		}
	}

	postFeatures := make(map[int64]model.PostFeatures, len(candidates))
	if repository == nil {
		degraded = true
	} else {
		postIDs := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			postIDs = append(postIDs, candidate.PostID)
		}
		loaded, err := repository.LoadPostFeatures(ctx, postIDs)
		if err != nil {
			degraded = true
		} else {
			postFeatures = loaded
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, degraded, err
	}

	result := make([]model.PostCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.PostID == excludedPostID || containsID(viewer.PositivePostIDs, candidate.PostID) ||
			containsID(viewer.NegativePostIDs, candidate.PostID) || containsID(viewer.SeenPostIDs, candidate.PostID) {
			continue
		}
		features := candidate.Features
		if loaded, ok := postFeatures[candidate.PostID]; ok && loaded.Known {
			features = loaded
		}
		if !features.Known || !features.Available || !isPublic(features.Visibility) {
			continue
		}
		if _, blocked := viewer.BlockedAuthors[features.AuthorID]; blocked && features.AuthorID > 0 {
			continue
		}
		candidate.Features = features
		candidate.AuthorID = features.AuthorID
		candidate.Category = features.Category
		result = append(result, candidate)
	}
	if len(result) == 0 && len(candidates) > 0 && degraded {
		return nil, true, fmt.Errorf("feature repository unavailable and no verified post candidates remain")
	}
	return result, degraded, nil
}

func enrichAndFilterUsers(
	ctx context.Context,
	repository model.FeatureRepository,
	requestUserID int64,
	candidates []model.UserCandidate,
) ([]model.UserCandidate, bool, error) {
	featuresByUser := make(map[int64]model.UserFeatures, len(candidates))
	degraded := false
	if repository == nil {
		degraded = true
	} else {
		userIDs := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			userIDs = append(userIDs, candidate.UserID)
		}
		loaded, err := repository.LoadUserFeatures(ctx, userIDs)
		if err != nil {
			degraded = true
		} else {
			featuresByUser = loaded
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, degraded, err
	}
	result := make([]model.UserCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.UserID == requestUserID {
			continue
		}
		features := candidate.Features
		if loaded, ok := featuresByUser[candidate.UserID]; ok && loaded.Known {
			features = loaded
		}
		if !features.Known || !features.Available || !isPublic(features.Visibility) {
			continue
		}
		candidate.Features = features
		candidate.Category = features.Category
		result = append(result, candidate)
	}
	if len(result) == 0 && len(candidates) > 0 && degraded {
		return nil, true, fmt.Errorf("feature repository unavailable and no verified user candidates remain")
	}
	return result, degraded, nil
}

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

func markPostDegradation(candidates []model.PostCandidate, degradation string) {
	if degradation == "" {
		return
	}
	for index := range candidates {
		candidates[index].ModelVersion = appendDegradation(candidates[index].ModelVersion, degradation)
	}
}

func markUserDegradation(candidates []model.UserCandidate, degradation string) {
	if degradation == "" {
		return
	}
	for index := range candidates {
		candidates[index].ModelVersion = appendDegradation(candidates[index].ModelVersion, degradation)
	}
}

func recommendationRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return errx.Wrap(err, errx.SystemError)
	}
	return errx.Wrap(err, errx.ServiceUnavailable)
}

func normalizedScene(scene string) string {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		return "home"
	}
	return scene
}

func validIdentity(userID int64, anonymousID string) bool {
	anonymousID = strings.TrimSpace(anonymousID)
	return userID >= 0 && len(anonymousID) <= 256 && (userID > 0 || anonymousID != "")
}

func validRequestMetadata(requestID, scene, sessionID, experimentID string) bool {
	requestID = strings.TrimSpace(requestID)
	return requestID != "" && len(requestID) <= 128 && len(strings.TrimSpace(scene)) <= 64 &&
		len(strings.TrimSpace(sessionID)) <= 128 && len(strings.TrimSpace(experimentID)) <= 128
}

func configuredPageSize(requested int32, c config.Config) (int, error) {
	pageSize := int(requested)
	if pageSize == 0 {
		pageSize = c.DefaultPageSize
		if pageSize <= 0 {
			pageSize = defaultPageSize
		}
	}
	maximum := c.MaxPageSize
	if maximum <= 0 {
		maximum = 50
	}
	if pageSize <= 0 || pageSize > maximum {
		return 0, errx.NewWithCode(errx.ParamError)
	}
	return pageSize, nil
}

func candidateLimit(pageSize int, c config.Config) int {
	multiplier := c.CandidateMultiplier
	if multiplier <= 0 {
		multiplier = defaultCandidateMultiple
	}
	return pageSize * multiplier
}

func cursorTTL(c config.Config) int {
	if c.CursorTTLSeconds > 0 {
		return c.CursorTTLSeconds
	}
	return defaultCursorTTLSeconds
}

func ruleModelVersion(c config.Config) string {
	if c.RuleModelVersion != "" {
		return c.RuleModelVersion
	}
	return "rules-v2"
}

func appendSource(current, next string) string {
	if next == "" || hasSource(current, next) {
		return current
	}
	if current == "" {
		return next
	}
	return current + "," + next
}

func hasSource(sources, source string) bool {
	for _, current := range strings.Split(sources, ",") {
		if current == source {
			return true
		}
	}
	return false
}

func appendDegradation(version, degradation string) string {
	if degradation == "" || strings.Contains(version, degradation) {
		return version
	}
	if version == "" {
		return degradation
	}
	return version + "+" + degradation
}

func emptyViewerFeatures() model.ViewerFeatures {
	return model.ViewerFeatures{
		PositivePostIDs: make(map[int64]struct{}),
		NegativePostIDs: make(map[int64]struct{}),
		SeenPostIDs:     make(map[int64]struct{}),
		BlockedAuthors:  make(map[int64]struct{}),
	}
}

func containsID(ids map[int64]struct{}, id int64) bool {
	_, exists := ids[id]
	return exists
}

func isPublic(visibility string) bool {
	return visibility == "" || strings.EqualFold(visibility, "public")
}

func boundedFeature(value float64) float64 {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value <= 1 {
		return value
	}
	return value / (1 + value)
}

func sortPostCandidates(candidates []model.PostCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FinalScore == candidates[j].FinalScore {
			return candidates[i].PostID < candidates[j].PostID
		}
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
}

func setPostModelVersion(candidates []model.PostCandidate, version string) {
	for index := range candidates {
		candidates[index].ModelVersion = version
	}
}

func deterministicJitter(seed string, id int64) float64 {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, id)))
	value := binary.BigEndian.Uint64(digest[:8])
	return float64(value%1000) / 1_000_000
}
